package main

import (
	"time"

	"github.com/aykevl/board"
	"github.com/aykevl/tinygl/pixel"
)

func main() {
	// Assert that board.Display implements board.Displayer.
	checkScreen(board.Display.Configure())

	// Assert that Display uses the usual interface.
	var _ interface {
		Size() (int16, int16)
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
}

func checkScreen[T pixel.Color](display board.Displayer[T]) {
}
