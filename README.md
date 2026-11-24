# deej 

with buttons and a volume curve

A fork of [omriharel/deej](https://github.com/omriharel/deej), the open-source hardware volume mixer for Windows. It lets physical sliders control the volume of individual apps. This fork adds buttons, automatic reconnecting, and an adjustable volume curve. The hardware is an Arduino Micro communicating to the PC over USB serial.

## Changes

- Auto-reconnect: deej keeps running with no board plugged in and connects if it shows up. It also recovers if the board is unplugged while running.
- Buttons: Each button sends a key press (e.g. F13-F17) mapped in `config.yaml`, which can be bound to anything.
- Volume curve: `volume_curve` in `config.yaml` modifies the slider response. `1.0` is linear.

See `config.yaml` for the new options.

## Hardware

Flash [`arduino/deej-6-sliders-5-buttons`](arduino/deej-6-sliders-5-buttons/deej-6-sliders-5-buttons.ino) to the board. The sketch sends lines like:

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

All credit for deej itself goes to [Omri Harel](https://github.com/omriharel).
