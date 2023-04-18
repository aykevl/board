package board

import "time"

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

var lastWaitForVBlank time.Time

// Utility function for all those boards that don't support vblank.
func dummyWaitForVBlank(defaultInterval time.Duration) {
	waitUntil := lastWaitForVBlank.Add(defaultInterval)
	now := time.Now()
	duration := waitUntil.Sub(now)
	if duration < 0 {
		lastWaitForVBlank = now
		return
	}
	time.Sleep(duration)
	lastWaitForVBlank = waitUntil
}

// Dummy implementation of the Power value, for devices with no battery or where
// the battery status cannot be read.
type dummyBattery struct {
	state ChargeState
}

func (b dummyBattery) Configure() {
	// nothing to do here
}

func (b dummyBattery) Status() (ChargeState, uint32) {
	return b.state, 0
}
