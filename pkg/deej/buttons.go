package deej

import (
	"go.uber.org/zap"
)

// buttonHandler turns button press events from the board into key presses
type buttonHandler struct {
	deej   *Deej
	logger *zap.SugaredLogger
}

func newButtonHandler(deej *Deej, logger *zap.SugaredLogger) *buttonHandler {
	return &buttonHandler{deej: deej, logger: logger.Named("buttons")}
}

func (b *buttonHandler) start() {
	events := b.deej.serial.SubscribeToButtonEvents()

	go func() {
		defer b.deej.recoverFromPanic()

		for event := range events {
			code, mapped := b.deej.config.ButtonMapping[event.ButtonID]
			if !mapped {
				b.logger.Debugw("Button has no mapping, ignoring", "button", event.ButtonID)
				continue
			}

			if err := sendKeyPress(code); err != nil {
				b.logger.Warnw("Failed to send key press", "button", event.ButtonID, "code", code, "error", err)
			} else {
				b.logger.Debugw("Sent key press", "button", event.ButtonID, "code", code)
			}
		}
	}()
}

// vkFromConfigCode converts a button_mapping value to a Windows virtual-key code.
// values above 0xFFF follow the keybd_event library's convention (virtual key + 0xFFF),
// e.g. F13 is 0x7C + 0xFFF = 4219
func vkFromConfigCode(code int) int {
	if code > 0xFFF {
		return code - 0xFFF
	}
	return code
}
