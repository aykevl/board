//go:build mch2022

package board

import (
	"machine"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/ili9341"
)

var (
	Display = mainDisplay{}
	Buttons = noButtons{}
)

type mainDisplay struct{}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
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

func (d mainDisplay) MaxBrightness() int {
	return 0
}

func (d mainDisplay) SetBrightness(level int) {
	// Brightness is controlled by the rp2040 chip.
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	// The FPGA has a parallel output and can probably do tear-free updates, but
	// not the ESP32.
	dummyWaitForVBlank(defaultInterval)
}

func (d mainDisplay) Size() (width, height int16) {
	return 320, 240
}

func (d mainDisplay) PPI() int {
	return 166 // 320px / (48.96mm / 25.4)
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	return noTouch{}
}
