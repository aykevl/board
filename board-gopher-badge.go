//go:build gopher_badge

package board

import (
	"machine"
	"math/bits"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/st7789"
)

var (
	Display = Display0
)

type internalSPI0 struct{ configured bool }

var InternalSPI0 = &internalSPI0{}

func (spi *internalSPI0) Get() *machine.SPI {
	if !spi.configured {
		machine.SPI0.Configure(machine.SPIConfig{
			// Mode 3 appears to be compatible with mode 0, but is slightly
			// faster: each byte takes 9 clock cycles instead of 10.
			// TODO: try to eliminate this last bit? Two ideas:
			//   - use 16-bit transfers, to halve the time the gap takes
			//   - use PIO, which apparently is able to send data without gap
			// It would seem like TI mode would be faster (it has no gap), but
			// it samples data on the falling edge instead of on the rising edge
			// like the st7789 expects.
			Mode:      3,
			SCK:       machine.SPI0_SCK_PIN,
			SDO:       machine.SPI0_SDO_PIN,
			SDI:       machine.SPI0_SDI_PIN,
			Frequency: 62_500_000, // datasheet for st7789 says 16ns (62.5MHz) is the max clock speed
		})
	}
	return machine.SPI0
}

var Display0 display0Config

type display0Config struct{}

func (d display0Config) Configure() Displayer[pixel.RGB565BE] {
	spi := InternalSPI0.Get()
	display := st7789.New(spi,
		machine.TFT_RST,       // TFT_RESET
		machine.TFT_WRX,       // TFT_DC
		machine.TFT_CS,        // TFT_CS
		machine.TFT_BACKLIGHT) // TFT_LITE

	display.Configure(st7789.Config{
		Rotation: st7789.ROTATION_90,
		Height:   320,
	})

	return &display
}

var Buttons = &gpioButtons{}

type gpioButtons struct {
	state         uint8
	previousState uint8
}

func (b *gpioButtons) Configure() {
	machine.BUTTON_A.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_B.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_UP.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_LEFT.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_DOWN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_RIGHT.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *gpioButtons) ReadInput() {
	state := uint8(0)
	if !machine.BUTTON_A.Get() {
		state |= 1
	}
	if !machine.BUTTON_B.Get() {
		state |= 2
	}
	if !machine.BUTTON_UP.Get() {
		state |= 4
	}
	if !machine.BUTTON_LEFT.Get() {
		state |= 8
	}
	if !machine.BUTTON_DOWN.Get() {
		state |= 16
	}
	if !machine.BUTTON_RIGHT.Get() {
		state |= 32
	}
	b.state = state
}

var codes = [8]Key{
	KeyA,
	KeyB,
	KeyUp,
	KeyLeft,
	KeyDown,
	KeyRight,
}

func (b *gpioButtons) NextEvent() KeyEvent {
	// The xor between the previous state and the current state is the buttons
	// that changed.
	change := b.state ^ b.previousState
	if change == 0 {
		return NoKeyEvent
	}

	// Find the index of the button with the lowest index that changed state.
	index := bits.TrailingZeros32(uint32(change))
	e := KeyEvent(codes[index])
	if b.state&(1<<index) == 0 {
		// The button state change was from 1 to 0, so it was released.
		e |= KeyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in b.state.
	b.previousState ^= (1 << index)

	return e
}
