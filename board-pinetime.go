//go:build pinetime_devkit0

package board

import (
	"device/nrf"
	"machine"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/st7789"
)

const (
	Name = "pinetime"

	touchInterruptPin = 28
)

var (
	Display = mainDisplay{}
	Buttons = &singleButton{}
)

func init() {
	// Enable the DC/DC regulator.
	// This doesn't affect sleep power consumption, but significantly reduces
	// runtime power consumpton of the CPU core (almost halving the current
	// required).
	nrf.POWER.DCDCEN.Set(nrf.POWER_DCDCEN_DCDCEN)
}

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
	// After the pin goes low (for a very short time), the touch controller is
	// accessible over I2C for as long as a finger touches the screen and a
	// short time afterwards (a second or so) before going back to sleep.
	//
	// We don't actually use an interrupt here because pin change interrupts
	// result in far too much current consumption (jumping from 0.19mA to
	// 0.65mA), probably due to anomaly 97:
	// https://infocenter.nordicsemi.com/index.jsp?topic=%2Ferrata_nRF52832_Rev2%2FERR%2FnRF52832%2FRev2%2Flatest%2Fanomaly_832_97.html
	// Also see:
	// https://devzone.nordicsemi.com/f/nordic-q-a/50624/about-current-consumption-of-gpio-and-gpiote
	// We could use a PORT interrupt in GPIOTE, using it as a level interrupt.
	// And it would be a good idea to implement this in TinyGo directly (as a
	// level interrupt), but in the meantime we'll use this quick-n-dirty hack.
	nrf.P0.PIN_CNF[touchInterruptPin].Set(nrf.GPIO_PIN_CNF_DIR_Input<<nrf.GPIO_PIN_CNF_DIR_Pos | nrf.GPIO_PIN_CNF_INPUT_Connect<<nrf.GPIO_PIN_CNF_INPUT_Pos | nrf.GPIO_PIN_CNF_SENSE_Low<<nrf.GPIO_PIN_CNF_SENSE_Pos)

	// Run I2C at a high speed (400KHz).
	touchI2C.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		SDA:       machine.Pin(6),
		SCL:       machine.Pin(7),
	})

	return touchInput{}
}

var touchI2C = machine.I2C1

var touchPoints [1]TouchPoint

type touchInput struct{}

var touchID uint32 = 1

var touchData = make([]byte, 6)

func (input touchInput) ReadTouch() []TouchPoint {
	// Read the bit from the LATCH reister, which is set to high when TP_INT
	// goes high but doesn't go low on its own. We do that manually once no more
	// touches are read from the touch controller.
	if nrf.P0.LATCH.Get()&(1<<touchInterruptPin) != 0 {
		touchI2C.ReadRegister(21, 1, touchData)
		num := touchData[1] & 0x0f
		if num == 0 {
			touchID++ // for the next time
			// Stop reading touch events.
			// There may be a small race condition here, if the touch controller
			// detects another touch while reading the touch data over I2C.
			nrf.P0.LATCH.Set(1 << touchInterruptPin)
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
	machine.BUTTON_OUT.Low()
	machine.BUTTON_IN.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *singleButton) ReadInput() {
	// BUTTON_OUT needs to be kept low most of the time to avoid a ~34µA current
	// increase. However, setting it to high just before reading doesn't appear
	// to be enough: a small delay is needed. This can be done by setting
	// BUTTON_OUT high multiple times in a row, which doesn't do anything except
	// introduce the needed delay.
	// Four stores appear to be enough to get readings, I have added a fifth to
	// be sure.
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	b.state = machine.BUTTON_IN.Get()
	machine.BUTTON_OUT.Low()
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
