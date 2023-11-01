package main

import (
	"time"

	"github.com/aykevl/board"
	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers"
)

func main() {
	// Verify board name constant.
	var _ string = board.Name

	// Assert that board.Display implements board.Displayer.
	checkScreen(board.Display.Configure())

	// Assert that Display uses the usual interface.
	var _ interface {
		//Configure() // already checked above
		PPI() int
		ConfigureTouch() board.TouchInput
		MaxBrightness() int
		SetBrightness(int)
		WaitForVBlank(time.Duration)
	} = board.Display

	// Assert that board.Buttons uses the usual interface.
	var _ interface {
		Configure()
		ReadInput()
		NextEvent() board.KeyEvent
	} = board.Buttons

	// Assert that board.Power uses the usual interface.
	var _ interface {
		Configure()
		Status() (state board.ChargeState, microvolts uint32, percent int8)
	} = board.Power

	// All sensors must implement the exact same interface, even if some methods
	// are unsupported.
	var _ interface {
		Configure(which drivers.Measurement) error
		Update(which drivers.Measurement) error
		Acceleration() (x, y, z int32)
		Steps() uint32
		Temperature() int32
	} = board.Sensors

	// Assert that board.AddressableLEDs uses the usual interface.
	var _ interface {
		Configure()
		Update()
	} = &board.AddressableLEDs
}

func checkScreen[T pixel.Color](display board.Displayer[T]) {
}
