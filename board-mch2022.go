//go:build mch2022

package board

import (
	"machine"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/ili9341"
)

var (
	Display = Display0
	Buttons = noButtons{}
)

var Display0 display0Config

type display0Config struct{}

func (d display0Config) Configure() Displayer[pixel.RGB565BE] {
	machine.LCD_MODE.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LCD_MODE.Low()

	machine.SPI2.Configure(machine.SPIConfig{
		Frequency: 80_000_000, // This is probably overclocking the ILI9341 but it seems to work.
		SCK:       18,
		SDO:       23,
		SDI:       35,
	})

	display := ili9341.NewSPI(machine.SPI2, machine.LCD_DC, machine.SPI0_CS_LCD_PIN, machine.LCD_RESET)
	display.Configure(ili9341.Config{
		Rotation: ili9341.Rotation90,
	})

	return display
}

func (d display0Config) Size() (width, height int16) {
	return 320, 240
}

func (d display0Config) PhysicalSize() (width, height int) {
	return 49, 37 // 48.96 x 36.72
}
