package board

// This file contains dummy devices, for devices which don't support a
// particular kind of device.

// Dummy button input that doesn't actually read any inputs.
// Used for boards that don't have any buttons.
type noButtons struct{}

func (b noButtons) Configure() {
}

func (b noButtons) ReadInput() {
}

func (b noButtons) NextEvent() KeyEvent {
	return NoKeyEvent
}

// Dummy touch object that doesn't read any input.
// Used for displays without touch capabilities.
type noTouch struct{}

func (t noTouch) ReadTouch() []TouchPoint {
	return nil
}
