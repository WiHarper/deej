package deej

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jacobsa/go-serial/serial"
	"go.uber.org/zap"

	"github.com/omriharel/deej/pkg/deej/util"
)

// SerialIO provides a deej-aware abstraction layer to managing serial I/O.
// It owns the connection lifecycle: it connects as soon as the configured port
// exists, notices when the board goes away, and reconnects on its own
type SerialIO struct {
	deej   *Deej
	logger *zap.SugaredLogger

	mu          sync.Mutex
	conn        io.ReadWriteCloser
	running     bool
	stopChannel chan struct{}
	connOptions serial.OpenOptions

	lastKnownNumSliders        int
	currentSliderPercentValues []float32

	buttonStates    []bool
	lastButtonPress []time.Time

	sliderMoveConsumers []chan SliderMoveEvent
	buttonConsumers     []chan ButtonEvent
}

// SliderMoveEvent represents a single slider move captured by deej
type SliderMoveEvent struct {
	SliderID     int
	PercentValue float32
}

// ButtonEvent represents a single button press captured by deej
type ButtonEvent struct {
	ButtonID int
}

const (
	// how long to wait between attempts to open the port while the board is away
	reconnectInterval = 2 * time.Second

	// the board streams constantly, so a long silence means the connection is dead
	readWatchdogTimeout = 5 * time.Second

	// ignore repeat presses of the same button closer together than this
	buttonDebounce = 30 * time.Millisecond
)

// accepts "s123|s456|b1|b0" (sliders prefixed s, buttons prefixed b, 0 = pressed)
// as well as the plain "123|456" format of stock deej
var expectedLinePattern = regexp.MustCompile(`^(s?\d{1,4}|b[01])(\|(s?\d{1,4}|b[01]))*\r\n$`)

// NewSerialIO creates a SerialIO instance that uses the provided deej
// instance's connection info to establish communications with the arduino chip
func NewSerialIO(deej *Deej, logger *zap.SugaredLogger) (*SerialIO, error) {
	logger = logger.Named("serial")

	sio := &SerialIO{
		deej:                deej,
		logger:              logger,
		stopChannel:         make(chan struct{}),
		sliderMoveConsumers: []chan SliderMoveEvent{},
		buttonConsumers:     []chan ButtonEvent{},
	}

	logger.Debug("Created serial i/o instance")

	// respond to config changes
	sio.setupOnConfigReload()

	return sio, nil
}

// Start begins connecting to our arduino chip in the background. It returns right away:
// if the board isn't plugged in, deej keeps running and connects as soon as it appears
func (sio *SerialIO) Start() error {
	sio.mu.Lock()
	defer sio.mu.Unlock()

	if sio.running {
		sio.logger.Warn("Already started, can't start another without stopping first")
		return errors.New("serial: already running")
	}

	sio.running = true
	go sio.connectionLoop(sio.stopChannel)

	return nil
}

// Stop signals us to shut down our serial connection and stop reconnecting
func (sio *SerialIO) Stop() {
	sio.mu.Lock()
	defer sio.mu.Unlock()

	if !sio.running {
		sio.logger.Debug("Not running, nothing to stop")
		return
	}

	sio.logger.Debug("Shutting down serial connection")
	sio.running = false
	close(sio.stopChannel)
	sio.stopChannel = make(chan struct{})

	if sio.conn != nil {
		sio.conn.Close()
	}
}

// SubscribeToSliderMoveEvents returns an unbuffered channel that receives
// a sliderMoveEvent struct every time a slider moves
func (sio *SerialIO) SubscribeToSliderMoveEvents() chan SliderMoveEvent {
	ch := make(chan SliderMoveEvent)
	sio.sliderMoveConsumers = append(sio.sliderMoveConsumers, ch)

	return ch
}

// SubscribeToButtonEvents returns a channel that receives a ButtonEvent every time a button is pressed
func (sio *SerialIO) SubscribeToButtonEvents() chan ButtonEvent {
	ch := make(chan ButtonEvent, 16)
	sio.buttonConsumers = append(sio.buttonConsumers, ch)

	return ch
}

func (sio *SerialIO) setupOnConfigReload() {
	configReloadedChannel := sio.deej.config.SubscribeToChanges()

	const stopDelay = 50 * time.Millisecond

	go func() {
		for range configReloadedChannel {

			// make any config reload unset our slider number to ensure process volumes are being re-set
			// (the next read line will emit SliderMoveEvent instances for all sliders)
			// this needs to happen after a small delay, because the session map will also re-acquire sessions
			// whenever the config file is reloaded, and we don't want it to receive these move events while the map
			// is still cleared. this is kind of ugly, but shouldn't cause any issues
			go func() {
				<-time.After(stopDelay)
				sio.lastKnownNumSliders = 0
			}()

			// if connection params have changed, drop the connection - the connection loop will reopen it
			sio.mu.Lock()
			changed := sio.conn != nil && (sio.deej.config.ConnectionInfo.COMPort != sio.connOptions.PortName ||
				uint(sio.deej.config.ConnectionInfo.BaudRate) != sio.connOptions.BaudRate)
			if changed {
				sio.logger.Info("Detected change in connection parameters, renewing connection")
				sio.conn.Close()
			}
			sio.mu.Unlock()
		}
	}()
}

// connectionLoop opens the port, reads from it until it fails, then starts over.
// it only exits when the stop channel is closed
func (sio *SerialIO) connectionLoop(stop chan struct{}) {
	waitingLogged := false

	for {
		conn, opts, err := sio.open()
		if err != nil {

			// stay quiet: a missing board is a normal state for a laptop
			if !waitingLogged {
				sio.logger.Infow("Board not available, will keep retrying",
					"comPort", opts.PortName, "error", err)
				waitingLogged = true
			}

			select {
			case <-stop:
				return
			case <-time.After(reconnectInterval):
				continue
			}
		}

		waitingLogged = false
		namedLogger := sio.logger.Named(strings.ToLower(opts.PortName))
		namedLogger.Infow("Connected", "conn", conn)

		sio.mu.Lock()
		sio.conn = conn
		sio.connOptions = opts
		sio.mu.Unlock()

		// make the next line re-send every slider position, and re-learn button states
		sio.lastKnownNumSliders = 0
		sio.buttonStates = nil

		stopped := sio.readUntilFailure(namedLogger, conn, stop)

		sio.mu.Lock()
		conn.Close()
		sio.conn = nil
		sio.mu.Unlock()

		if stopped {
			return
		}

		namedLogger.Info("Connection lost, will keep retrying")

		select {
		case <-stop:
			return
		case <-time.After(reconnectInterval):
		}
	}
}

func (sio *SerialIO) open() (io.ReadWriteCloser, serial.OpenOptions, error) {

	// set minimum read size according to platform (0 for windows, 1 for linux)
	// this prevents a rare bug on windows where serial reads get congested,
	// resulting in significant lag
	minimumReadSize := 0
	if util.Linux() {
		minimumReadSize = 1
	}

	opts := serial.OpenOptions{
		PortName:        sio.deej.config.ConnectionInfo.COMPort,
		BaudRate:        uint(sio.deej.config.ConnectionInfo.BaudRate),
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: uint(minimumReadSize),
	}

	sio.logger.Debugw("Attempting serial connection",
		"comPort", opts.PortName,
		"baudRate", opts.BaudRate,
		"minReadSize", minimumReadSize)

	conn, err := serial.Open(opts)
	if err != nil {
		return nil, opts, fmt.Errorf("open serial connection: %w", err)
	}

	return conn, opts, nil
}

// readUntilFailure handles lines until the connection fails, goes silent or we're stopped.
// returns true if it ended because of a stop request
func (sio *SerialIO) readUntilFailure(logger *zap.SugaredLogger, conn io.Reader, stop chan struct{}) bool {
	lines := make(chan string)
	failed := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				failed <- err
				return
			}

			select {
			case lines <- line:
			case <-done:
				return
			}
		}
	}()

	watchdog := time.NewTimer(readWatchdogTimeout)
	defer watchdog.Stop()

	for {
		select {
		case <-stop:
			return true
		case err := <-failed:
			logger.Warnw("Failed to read from serial", "error", err)
			return false
		case <-watchdog.C:
			logger.Warn("No data from board, treating connection as dead")
			return false
		case line := <-lines:
			if !watchdog.Stop() {
				select {
				case <-watchdog.C:
				default:
				}
			}
			watchdog.Reset(readWatchdogTimeout)

			if sio.deej.Verbose() {
				logger.Debugw("Read new line", "line", line)
			}

			sio.handleLine(logger, line)
		}
	}
}

func (sio *SerialIO) handleLine(logger *zap.SugaredLogger, line string) {

	// this function receives an unsanitized line which is guaranteed to end with LF,
	// but most lines will end with CRLF. it may also have garbage instead of
	// deej-formatted values, so we must check for that! just ignore bad ones
	if !expectedLinePattern.MatchString(line) {
		return
	}

	// trim the suffix
	line = strings.TrimSuffix(line, "\r\n")

	// split on pipe (|) into slider values ("s512" or "512", 0-1023) and button states ("b0"/"b1")
	var sliderFields []string
	var buttonStates []bool
	for _, field := range strings.Split(line, "|") {
		if strings.HasPrefix(field, "b") {

			// buttons are active-low: 0 means pressed
			buttonStates = append(buttonStates, field == "b0")
		} else {
			sliderFields = append(sliderFields, strings.TrimPrefix(field, "s"))
		}
	}

	// the first line could come out dirty sometimes (i.e. "4558|925|41|643|220"), so let's
	// check the first slider for correctness before trusting anything in this line
	if len(sliderFields) > 0 {
		if first, _ := strconv.Atoi(sliderFields[0]); first > 1023 {
			sio.logger.Debugw("Got malformed line from serial, ignoring", "line", line)
			return
		}
	}

	sio.handleSliders(logger, sliderFields)
	sio.handleButtons(logger, buttonStates)
}

func (sio *SerialIO) handleSliders(logger *zap.SugaredLogger, sliderFields []string) {
	numSliders := len(sliderFields)
	if numSliders == 0 {
		return
	}

	// update our slider count, if needed - this will send slider move events for all
	if numSliders != sio.lastKnownNumSliders {
		logger.Infow("Detected sliders", "amount", numSliders)
		sio.lastKnownNumSliders = numSliders
		sio.currentSliderPercentValues = make([]float32, numSliders)

		// reset everything to be an impossible value to force the slider move event later
		for idx := range sio.currentSliderPercentValues {
			sio.currentSliderPercentValues[idx] = -1.0
		}
	}

	// for each slider:
	moveEvents := []SliderMoveEvent{}
	for sliderIdx, stringValue := range sliderFields {

		// convert string values to integers ("1023" -> 1023)
		number, _ := strconv.Atoi(stringValue)

		// map the value from raw to a "dirty" float between 0 and 1 (e.g. 0.15451...)
		dirtyFloat := float32(number) / 1023.0

		// normalize it to an actual volume scalar between 0.0 and 1.0 with 2 points of precision
		normalizedScalar := util.NormalizeScalar(dirtyFloat)

		// if sliders are inverted, take the complement of 1.0
		if sio.deej.config.InvertSliders {
			normalizedScalar = 1 - normalizedScalar
		}

		// check if it changes the desired state (could just be a jumpy raw slider value)
		if util.SignificantlyDifferent(sio.currentSliderPercentValues[sliderIdx], normalizedScalar, sio.deej.config.NoiseReductionLevel) {

			// if it does, update the saved (physical) value and create a move event
			// carrying the volume after the response curve is applied
			sio.currentSliderPercentValues[sliderIdx] = normalizedScalar

			moveEvents = append(moveEvents, SliderMoveEvent{
				SliderID:     sliderIdx,
				PercentValue: util.ApplyVolumeCurve(normalizedScalar, sio.deej.config.VolumeCurve),
			})

			if sio.deej.Verbose() {
				logger.Debugw("Slider moved", "event", moveEvents[len(moveEvents)-1])
			}
		}
	}

	// deliver move events if there are any, towards all potential consumers
	if len(moveEvents) > 0 {
		for _, consumer := range sio.sliderMoveConsumers {
			for _, moveEvent := range moveEvents {
				consumer <- moveEvent
			}
		}
	}
}

func (sio *SerialIO) handleButtons(logger *zap.SugaredLogger, pressed []bool) {
	if len(pressed) == 0 {
		return
	}

	// first line after connecting (or a change in button count) only learns the current state,
	// so a button held down during a reconnect doesn't fire
	if len(pressed) != len(sio.buttonStates) {
		logger.Infow("Detected buttons", "amount", len(pressed))
		sio.buttonStates = pressed
		sio.lastButtonPress = make([]time.Time, len(pressed))
		return
	}

	now := time.Now()
	for buttonIdx, isPressed := range pressed {
		wasPressed := sio.buttonStates[buttonIdx]
		sio.buttonStates[buttonIdx] = isPressed

		// only react to the moment a button goes down
		if !isPressed || wasPressed {
			continue
		}

		if now.Sub(sio.lastButtonPress[buttonIdx]) < buttonDebounce {
			continue
		}
		sio.lastButtonPress[buttonIdx] = now

		if sio.deej.Verbose() {
			logger.Debugw("Button pressed", "button", buttonIdx)
		}

		for _, consumer := range sio.buttonConsumers {
			select {
			case consumer <- ButtonEvent{ButtonID: buttonIdx}:
			default:
				logger.Warnw("Button event consumer is backed up, dropping event", "button", buttonIdx)
			}
		}
	}
}
