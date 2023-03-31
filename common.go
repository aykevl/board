package board

import (
	"github.com/aykevl/tinygl/pixel"
)

var Simulator = struct {
	WindowTitle  string
	WindowWidth  int
	WindowHeight int
}{
	WindowTitle:  "Simulator",
	WindowWidth:  240,
	WindowHeight: 240,
}

type Displayer[T pixel.Color] interface {
	Size() (int16, int16)
	DrawRGBBitmap8(x, y int16, buf []uint8, w, h int16) error
	Display() error
}

type KeyEvent uint16

type Key uint8

const (
	NoKey = iota

	KeyLeft
	KeyRight
	KeyUp
	KeyDown

	KeySpace
	KeyA
	KeyB
	KeyL
	KeyR

	// Special keys, used on some boards.
	KeySelect
	KeyStart

	keyCustomStart // first custom key
)

const (
	NoKeyEvent KeyEvent = iota // No key event was available.

	KeyReleased = KeyEvent(1 << 15) // The uppoer bit is set when this is a release event
)

func (k KeyEvent) Key() Key {
	return Key(k) // lower 8 bits are the key code
}

func (k KeyEvent) Pressed() bool {
	return k&KeyReleased == 0
}

// Dummy button input that doesn't actually read any inputs.
// Used for boards that don't have any buttons.
type noButtons struct{}

func (b noButtons) Configure() {
}

func (b noButtons) ReadInput() {
}

func (b noButtons) NextEvent() KeyEvent {
	return NoKeyEvent
}
