//go:build badger2040

package board

import (
	"machine"
	"math/bits"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/uc8151"
)

const (
	Name = "badger2040"
)

var (
	Power   = dummyBattery{state: UnknownBattery}
	Sensors = baseSensors{}
	Display = mainDisplay{}
	Buttons = &gpioButtons{}
)

type mainDisplay struct{}

func (d mainDisplay) PPI() int {
	return 102 // 296px wide display / 2.9 inches wide display
}

func (d mainDisplay) Configure() Displayer[pixel.Monochrome] {
	machine.ENABLE_3V3.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.ENABLE_3V3.High()

	machine.SPI0.Configure(machine.SPIConfig{
		Frequency: 12 * machine.MHz,
		SCK:       machine.EPD_SCK_PIN,
		SDO:       machine.EPD_SDO_PIN,
	})

	display := uc8151.New(machine.SPI0, machine.EPD_CS_PIN, machine.EPD_DC_PIN, machine.EPD_RESET_PIN, machine.EPD_BUSY_PIN)
	display.Configure(uc8151.Config{
		Rotation:    drivers.Rotation270,
		Speed:       uc8151.TURBO,
		FlickerFree: true,
		Blocking:    false,
	})

	display.ClearDisplay()

	return &display
}

func (d mainDisplay) MaxBrightness() int {
	return 1
}

func (d mainDisplay) SetBrightness(level int) {
	// Nothing to do here.
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	dummyWaitForVBlank(defaultInterval)
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	return noTouch{}
}

type gpioButtons struct {
	state         uint8
	previousState uint8
}

func (b *gpioButtons) Configure() {
	machine.BUTTON_A.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_B.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_C.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_UP.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_DOWN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_USER.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *gpioButtons) ReadInput() {
	state := uint8(0)
	if !machine.BUTTON_A.Get() {
		state |= 1
	}
	if !machine.BUTTON_B.Get() {
		state |= 2
	}
	if !machine.BUTTON_C.Get() {
		state |= 4
	}
	if !machine.BUTTON_UP.Get() {
		state |= 8
	}
	if !machine.BUTTON_DOWN.Get() {
		state |= 16
	}
	if !machine.BUTTON_USER.Get() {
		state |= 32
	}
	b.state = state
}

var codes = [8]Key{
	KeyA,
	KeyB,
	KeyRight,
	KeyUp,
	KeyDown,
	KeyLeft,
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
		e |= keyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in b.state.
	b.previousState ^= (1 << index)

	return e
}
