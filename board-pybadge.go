//go:build pybadge

package board

import (
	"machine"
	"math/bits"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/shifter"
	"tinygo.org/x/drivers/st7735"
)

var (
	Display = mainDisplay{}
	Buttons = &buttonsConfig{}
)

type mainDisplay struct{}

func (d mainDisplay) Size() (width, height int16) {
	return 160, 128
}

func (d mainDisplay) PhysicalSize() (width, height int) {
	return 36, 29 // size in millimeters
}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	machine.SPI1.Configure(machine.SPIConfig{
		SCK:       machine.SPI1_SCK_PIN,
		SDO:       machine.SPI1_SDO_PIN,
		SDI:       machine.SPI1_SDI_PIN,
		Frequency: 15_000_000, // datasheet for st7735 says 66ns (~15.15MHz) is the max speed
	})

	display := st7735.New(machine.SPI1, machine.TFT_RST, machine.TFT_DC, machine.TFT_CS, machine.TFT_LITE)
	display.Configure(st7735.Config{
		Rotation: st7735.ROTATION_90,
	})
	return &display
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	dummyWaitForVBlank(defaultInterval)
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	return noTouch{}
}

type buttonsConfig struct {
	shifter.Device
	lastState, currentState uint8
}

func (b *buttonsConfig) Configure() {
	b.Device = shifter.NewButtons()
	b.Device.Configure()
}

func (b *buttonsConfig) ReadInput() {
	b.currentState, _ = b.Device.ReadInput()
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

func (b *buttonsConfig) NextEvent() KeyEvent {
	// The xor between the previous state and the current state is the buttons
	// that changed.
	change := b.currentState ^ b.lastState
	if change == 0 {
		return NoKeyEvent
	}

	// Find the index of the button with the lowest index that changed state.
	index := bits.TrailingZeros32(uint32(change))
	e := KeyEvent(codes[index])
	if b.currentState&(1<<index) == 0 {
		// The button state change was from 1 to 0, so it was released.
		e |= keyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in currentState.
	b.lastState ^= (1 << index)

	return e
}
