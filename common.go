package board

import (
	"github.com/aykevl/tinygl/pixel"
)

// Settings for the simulator. These can be modified at any time, but it is
// recommended to modify them before configuring any of the board peripherals.
//
// These can be modified to match whatever board your main target is. For
// example, if your board has a display that's only 160 by 128 pixels, you can
// modify the window size here to get a realistic simulation.
var Simulator = struct {
	WindowTitle string

	// Width and height in virtual pixels (matching Size()). The window will
	// take up more physical pixels on high-DPI screens.
	WindowWidth  int
	WindowHeight int

	// Pixels per inch. The default is 120, which matches many commonly used
	// high-DPI screens (for example, Apple screens).
	WindowPPI int
}{
	WindowTitle:  "Simulator",
	WindowWidth:  240,
	WindowHeight: 240,
	WindowPPI:    120, // common on many modern displays (for example Retina is 254 / 2 = 127)
}

// The display interface shared by all supported displays.
type Displayer[T pixel.Color] interface {
	// The display size in pixels. This must match Display.Size().
	Size() (width, height int16)

	// Write data to the display in the usual row-major order (which matches the
	// usual order of text on a page: first left to right and then each line top
	// to bottom).
	//
	// TODO: this interface is likely to change because it requires unsafely
	// casting a []T to []uint8 in user code.
	DrawRGBBitmap8(x, y int16, buf []uint8, w, h int16) error

	// Display the written image on screen. This call may or may not be
	// necessary depending on the screen, but it's better to call it anyway.
	Display() error
}

// Key is a single keyboard key (not to be confused with a single character).
type Key uint8

// List of all supported key codes.
const (
	NoKey = iota

	// Special keys.
	KeyEscape

	// Navigation keys.
	KeyLeft
	KeyRight
	KeyUp
	KeyDown

	// Character keys.
	KeyEnter
	KeySpace
	KeyA
	KeyB
	KeyL
	KeyR

	// Special keys, used on some boards.
	KeySelect
	KeyStart
)

// KeyEvent is a single key press or release event.
type KeyEvent uint16

const (
	NoKeyEvent KeyEvent = iota // No key event was available.

	keyReleased = KeyEvent(1 << 15) // The upper bit is set when this is a release event
)

// Key returns the key code for this key event.
func (k KeyEvent) Key() Key {
	return Key(k) // lower 8 bits are the key code
}

// Pressed returns whether this event indicates a key press event. It returns
// true for a press, false for a release.
func (k KeyEvent) Pressed() bool {
	return k&keyReleased == 0
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
