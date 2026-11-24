package deej

import (
	"syscall"
	"time"
)

var procKeybdEvent = syscall.NewLazyDLL("user32.dll").NewProc("keybd_event")

const keyEventFKeyUp = 0x0002

// sendKeyPress presses and releases the key with the given button_mapping code
func sendKeyPress(code int) error {
	vk := uintptr(vkFromConfigCode(code))

	procKeybdEvent.Call(vk, 0, 0, 0)
	time.Sleep(10 * time.Millisecond)
	procKeybdEvent.Call(vk, 0, keyEventFKeyUp, 0)

	return nil
}
