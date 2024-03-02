//go:build thumby

package board

import (
	"machine"
	"math/bits"
	"time"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/ssd1306"
)

const (
	Name = "pybadge"
)

var (
	Power   = dummyBattery{state: UnknownBattery}
	Sensors = baseSensors{}
	Display = mainDisplay{}
	Buttons = &gpioButtons{}
)

type mainDisplay struct{}

func (d mainDisplay) PPI() int {
	return 192 // 72px wide display / 3/8 of an inch wide display
}

func (d mainDisplay) Configure() Displayer[pixel.Monochrome] {
	machine.SPI0.Configure(machine.SPIConfig{})
	display := ssd1306.NewSPI(machine.SPI0, machine.THUMBY_DC_PIN, machine.THUMBY_RESET_PIN, machine.THUMBY_CS_PIN)
	display.Configure(ssd1306.Config{
		Width:     72,
		Height:    40,
		ResetCol:  ssd1306.ResetValue{28, 99},
		ResetPage: ssd1306.ResetValue{0, 5},
	})

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
	machine.THUMBY_BTN_A_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.THUMBY_BTN_B_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.THUMBY_BTN_UDPAD_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.THUMBY_BTN_LDPAD_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.THUMBY_BTN_DDPAD_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.THUMBY_BTN_RDPAD_PIN.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *gpioButtons) ReadInput() {
	state := uint8(0)
	if !machine.THUMBY_BTN_A_PIN.Get() {
		state |= 1
	}
	if !machine.THUMBY_BTN_B_PIN.Get() {
		state |= 2
	}
	if !machine.THUMBY_BTN_UDPAD_PIN.Get() {
		state |= 4
	}
	if !machine.THUMBY_BTN_LDPAD_PIN.Get() {
		state |= 8
	}
	if !machine.THUMBY_BTN_DDPAD_PIN.Get() {
		state |= 16
	}
	if !machine.THUMBY_BTN_RDPAD_PIN.Get() {
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
		e |= keyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in b.state.
	b.previousState ^= (1 << index)

	return e
}
