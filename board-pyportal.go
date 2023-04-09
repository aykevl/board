//go:build pyportal

package board

import (
	"machine"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/ili9341"
)

var (
	Display = mainDisplay{}
	Buttons = noButtons{}
)

type mainDisplay struct{}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	// Enable and configure display.
	display := ili9341.NewParallel(
		machine.LCD_DATA0,
		machine.TFT_WR,
		machine.TFT_DC,
		machine.TFT_CS,
		machine.TFT_RESET,
		machine.TFT_RD,
	)
	display.Configure(ili9341.Config{
		Rotation: ili9341.Rotation270,
	})

	// Enable backlight.
	// TODO: do this in a separate method (and disable the backlight at
	// startup).
	backlight := machine.TFT_BACKLIGHT
	backlight.Configure(machine.PinConfig{machine.PinOutput})
	backlight.High()

	return display
}

func (d mainDisplay) Size() (width, height int16) {
	return 320, 240
}

func (d mainDisplay) PhysicalSize() (width, height int) {
	return 49, 37
}
