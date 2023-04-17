//go:build pinetime_devkit0

package board

import (
	"machine"
	"runtime/volatile"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/st7789"
)

const (
	Name = "pinetime"
)

var (
	Display = mainDisplay{}
	Buttons = &singleButton{}
)

type mainDisplay struct{}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	// Set the chip select line for the flash chip to inactive.
	cs := machine.Pin(5) // SPI CS
	cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	cs.High()

	// Configure the SPI bus.
	// TODO: use RGB444 for better performance
	spi := machine.SPI0
	spi.Configure(machine.SPIConfig{
		Frequency: 8_000_000, // 8MHz is the maximum the nrf52832 supports
		SCK:       machine.SPI0_SCK_PIN,
		SDO:       machine.SPI0_SDO_PIN,
		SDI:       machine.SPI0_SDI_PIN,
		Mode:      3,
	})

	// Configure the display.
	display := st7789.New(spi,
		machine.LCD_RESET,
		machine.LCD_RS, // data/command
		machine.LCD_CS,
		machine.LCD_BACKLIGHT_HIGH) // TODO: allow better backlight control
	display.Configure(st7789.Config{
		Width:     240,
		Height:    240,
		Rotation:  st7789.ROTATION_180,
		RowOffset: 80,
	})
	display.EnableBacklight(true) // disable the backlight

	return &display
}

func (d mainDisplay) MaxBrightness() int {
	return 1 // TODO: 0-7 is supported
}

func (d mainDisplay) SetBrightness(level int) {
	machine.LCD_BACKLIGHT_HIGH.Set(!(level > 0)) // low means on, high means off
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	dummyWaitForVBlank(defaultInterval)
}

func (d mainDisplay) Size() (width, height int16) {
	return 240, 240
}

func (d mainDisplay) PPI() int {
	return 261
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	// Configure touch interrupt pin.
	// After the interrupt is received, the touch controller is accessible over
	// I2C for as long as a finger touches the screen and a short time
	// afterwards (a few hundred milliseconds) before going back to sleep.
	intr := machine.Pin(28)
	intr.Configure(machine.PinConfig{Mode: machine.PinInput})
	intr.SetInterrupt(machine.PinFalling, handleTouchIntr)

	// Run I2C at a high speed (400KHz).
	touchI2C.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		SDA:       machine.Pin(6),
		SCL:       machine.Pin(7),
	})

	return touchInput{}
}

var touchI2C = machine.I2C1

var touchState volatile.Register8

func handleTouchIntr(machine.Pin) {
	touchState.Set(1)
}

var touchPoints [1]TouchPoint

type touchInput struct{}

var touchID uint32 = 1

var touchData = make([]byte, 6)

func (input touchInput) ReadTouch() []TouchPoint {
	state := touchState.Get()
	if state != 0 {
		touchI2C.ReadRegister(21, 1, touchData)
		num := touchData[1] & 0x0f
		if num == 0 {
			// TODO: there is a race condition here with the pin interrupt
			touchState.Set(0)
			touchID++ // for the next time
		}
		x := (uint16(touchData[2]&0xf) << 8) | uint16(touchData[3]) // x coord
		y := (uint16(touchData[4]&0xf) << 8) | uint16(touchData[5]) // y coord
		// TODO: account for screen rotation
		if x < 0 {
			x = 0
		}
		if x >= 240 {
			x = 240
		}
		if y < 0 {
			y = 0
		}
		if y >= 240 {
			y = 240
		}
		touchPoints[0] = TouchPoint{
			X:  int16(x),
			Y:  int16(y),
			ID: touchID,
		}
		return touchPoints[:1]
	}
	return nil
}

// State for the one and only button on the PineTime.
type singleButton struct {
	state         bool
	previousState bool
}

func (b *singleButton) Configure() {
	// BUTTON_OUT must be held high for BUTTON_IN to read anything useful.
	machine.BUTTON_OUT.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BUTTON_OUT.High()
	machine.BUTTON_IN.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *singleButton) ReadInput() {
	b.state = machine.BUTTON_IN.Get()
}

func (b *singleButton) NextEvent() KeyEvent {
	if b.state == b.previousState {
		return NoKeyEvent
	}
	e := KeyEvent(KeyEnter)
	if !b.state {
		e |= keyReleased
	}
	b.previousState = b.state
	return e
}
