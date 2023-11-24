//go:build mch2022

package board

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/ili9341"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/ws2812"
)

const (
	Name = "mch2022"
)

var (
	Power   = dummyBattery{state: UnknownBattery} // unimplemented
	Sensors = baseSensors{}
	Display = mainDisplay{}
	Buttons = noButtons{}
)

func init() {
	AddressableLEDs = &ws2812LEDs{}
}

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

func (d mainDisplay) PPI() int {
	return 166 // 320px / (48.96mm / 25.4)
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	return noTouch{}
}

type ws2812LEDs struct {
	data [5]colorGRB
}

func (l *ws2812LEDs) Configure() {
	// Enable power to the LEDs
	power := machine.PowerOn
	power.Configure(machine.PinConfig{Mode: machine.PinOutput})
	power.High()

	// Initialize the WS2812 data pin.
	machine.WS2812.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func (l *ws2812LEDs) Len() int {
	return len(l.data)
}

func (l *ws2812LEDs) SetRGB(i int, r, g, b uint8) {
	l.data[i] = colorGRB{
		R: r,
		G: g,
		B: b,
	}
}

// Send pixel data to the LEDs.
func (l *ws2812LEDs) Update() {
	ws := ws2812.Device{Pin: machine.WS2812}
	ws.Write(pixelsToBytes(l.data[:]))
}
