package deej

import "errors"

func sendKeyPress(code int) error {
	return errors.New("button key presses are only supported on Windows")
}
