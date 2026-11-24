# deej (fork with buttons)

A fork of [omriharel/deej](https://github.com/omriharel/deej), the open-source hardware volume mixer for Windows. It lets physical sliders control the volume of individual apps. This fork adds buttons, automatic reconnecting, and an adjustable volume curve. The hardware is an Arduino Micro talking to the PC over USB serial.

## What's different from upstream

- **Auto-reconnect.** deej keeps running when the board isn't plugged in and connects as soon as it shows up. It also recovers if the board is unplugged while running.
- **Buttons.** Each button sends a key press (e.g. F13-F17) mapped in `config.yaml`, which you can bind to anything else (macros, mute, etc.).
- **Volume curve.** `volume_curve` in `config.yaml` reshapes the sliders so the top of their range isn't cramped. `1.0` is linear (stock behavior); try `2.0`.

Slider mapping, `com_port` and the other settings work as in upstream. See `config.yaml` for the new options.

## Hardware

Flash [`arduino/deej-6-sliders-5-buttons`](arduino/deej-6-sliders-5-buttons/deej-6-sliders-5-buttons.ino) to the board. Check the slider and button pins at the top of the sketch against your wiring. Buttons are wired between a pin and ground. The sketch sends lines like:

```
s512|s1023|s0|s300|s877|s41|b1|b1|b0|b1|b1
```

Sliders are `s0`-`s1023`; buttons are `b1` released and `b0` pressed.

## Build and run

Requires [Go](https://go.dev/dl/) on Windows.

```
go build -ldflags "-H=windowsgui" -o deej.exe ./pkg/deej/cmd
```

Put `config.yaml` next to `deej.exe` and run it.

## License

MIT, same as upstream. See [LICENSE](LICENSE). All credit for deej itself goes to [Omri Harel](https://github.com/omriharel).
