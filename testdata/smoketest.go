package main

import (
	"time"

	"github.com/aykevl/board"
	"github.com/aykevl/tinygl/pixel"
)

func main() {
	// Verify board name constant.
	var _ string = board.Name

	// Assert that board.Display implements board.Displayer.
	checkScreen(board.Display.Configure())

	// Assert that Display uses the usual interface.
	var _ interface {
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

	// Assert that board.AddressableLEDs uses the usual interface.
	var _ interface {
		Configure()
		Update()
	} = &board.AddressableLEDs
}

func checkScreen[T pixel.Color](display board.Displayer[T]) {
}
