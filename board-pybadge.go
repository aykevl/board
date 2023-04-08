//go:build pybadge

package board

import (
	"machine"
	"math/bits"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/shifter"
	"tinygo.org/x/drivers/st7735"
)

var (
	Display = Display0
)

type internalSPI0 struct{ configured bool }

var InternalSPI0 = &internalSPI0{}

func (spi *internalSPI0) Get() machine.SPI {
	if !spi.configured {
		machine.SPI1.Configure(machine.SPIConfig{
			SCK:       machine.SPI1_SCK_PIN,
			SDO:       machine.SPI1_SDO_PIN,
			SDI:       machine.SPI1_SDI_PIN,
			Frequency: 15_000_000, // datasheet for st7735 says 66ns (~15.15MHz) is the max speed
		})
	}
	return machine.SPI1
}

var Display0 display0Config

type display0Config struct{}

func (d display0Config) Size() (width, height int) {
	return 160, 128
}

func (d display0Config) PhysicalSize() (width, height int) {
	return 36, 29 // size in millimeters
}

func (d display0Config) Configure() Displayer[pixel.RGB565BE] {
	spi := InternalSPI0.Get()
	display := st7735.New(spi, machine.TFT_RST, machine.TFT_DC, machine.TFT_CS, machine.TFT_LITE)
	display.Configure(st7735.Config{
		Rotation: st7735.ROTATION_90,
	})
	return &display
}

var shifterButtons shifter.Device

type buttonsConfig struct{}

var Buttons buttonsConfig

var lastButtonState, currentButtonState uint8

func (b buttonsConfig) Configure() {
	shifterButtons = shifter.NewButtons()
	shifterButtons.Configure()
}

func (b buttonsConfig) ReadInput() {
	state, _ := shifterButtons.ReadInput()
	currentButtonState = state
}

var codes = [8]Key{
	KeyLeft,
	KeyUp,
	KeyDown,
	KeyRight,
	KeySelect,
	KeyStart,
	KeyA,
	KeyB,
}

func (b buttonsConfig) NextEvent() KeyEvent {
	// The xor between the previous state and the current state is the buttons
	// that changed.
	change := currentButtonState ^ lastButtonState
	if change == 0 {
		return NoKeyEvent
	}

	// Find the index of the button with the lowest index that changed state.
	index := bits.TrailingZeros32(uint32(change))
	e := KeyEvent(codes[index])
	if currentButtonState&(1<<index) == 0 {
		// The button state change was from 1 to 0, so it was released.
		e |= KeyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in currentButtonState.
	lastButtonState ^= (1 << index)

	return e
}
