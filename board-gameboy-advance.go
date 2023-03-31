//go:build gameboyadvance

package board

import (
	"device/gba"
	"machine"
	"math/bits"

	"github.com/aykevl/tinygl/pixel"
)

var (
	Display = Display0
)

var Display0 display0Config

type display0Config struct{}

func (d display0Config) Size() (width, height int) {
	return 240, 160
}

func (d display0Config) PhysicalSize() (width, height int) {
	return 62, 41 // size in millimeters
}

func (d display0Config) Configure() Displayer[pixel.RGB555] {
	display := machine.Display
	display.Configure()
	return display
}

var Buttons = &gbaButtons{}

type gbaButtons struct {
	state         uint16
	previousState uint16
}

func (b *gbaButtons) Configure() {
	// nothing to configure
}

func (b *gbaButtons) ReadInput() {
	b.state = gba.KEY.INPUT.Get() ^ 0x3ff
}

var codes = [16]Key{
	KeyA,
	KeyB,
	KeySelect,
	KeyStart,
	KeyRight,
	KeyLeft,
	KeyUp,
	KeyDown,
	KeyR,
	KeyL,
}

func (b *gbaButtons) NextEvent() KeyEvent {
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
