//go:build gameboyadvance

package board

import (
	"device/gba"
	"errors"
	"math/bits"
	"runtime/volatile"
	"time"
	"unsafe"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers"
)

const (
	Name = "gameboy-advance"
)

var (
	Power           = dummyBattery{state: UnknownBattery}
	Display         = mainDisplay{}
	Buttons         = &gbaButtons{}
	AddressableLEDs = dummyAddressableLEDs{}
)

type mainDisplay struct{}

func (d mainDisplay) PPI() int {
	return 99
}

func (d mainDisplay) Configure() Displayer[pixel.RGB555] {
	// Use video mode 3 (in BG2, a 16bpp bitmap in VRAM) and Enable BG2.
	gba.DISP.DISPCNT.Set(gba.DISPCNT_BGMODE_3<<gba.DISPCNT_BGMODE_Pos |
		gba.DISPCNT_SCREENDISPLAY_BG2_ENABLE<<gba.DISPCNT_SCREENDISPLAY_BG2_Pos)
	return gbaDisplay{}
}

func (d mainDisplay) MaxBrightness() int {
	return 0
}

func (d mainDisplay) SetBrightness(level int) {
	// The display doesn't have a backlight.
}

func (d mainDisplay) WaitForVBlank(time.Duration) {
	// Wait until the VBlank flag is set.
	// TODO: sleep until the next VBlank instead of busy waiting.
	// (See VBlankIntrWait)
	for gba.DISP.DISPSTAT.Get()&(1<<gba.DISPSTAT_VBLANK_Pos) == 0 {
	}
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	return noTouch{}
}

type gbaDisplay struct{}

var displayFrameBuffer = (*[160 * 240]volatile.Register16)(unsafe.Pointer(uintptr(gba.MEM_VRAM)))

func (d gbaDisplay) Size() (x, y int16) {
	return 240, 160
}

func (d gbaDisplay) Display() error {
	// Nothing to do here.
	return nil
}

func (d gbaDisplay) DrawRGBBitmap8(x, y int16, buf []byte, width, height int16) error {
	for bufY := 0; bufY < int(height); bufY++ {
		for bufX := 0; bufX < int(width); bufX++ {
			index := bufY*int(width) + bufX
			val := uint16(buf[index*2+0]) + uint16(buf[index*2+1])<<8
			displayFrameBuffer[(int(y)+bufY)*240+int(x)+bufX].Set(val)
		}
	}
	return nil
}

func (d gbaDisplay) Sleep(sleepEnabled bool) error {
	return nil // nothign to do here
}

var errNoRotation = errors.New("error: SetRotation isn't supported")

func (d gbaDisplay) Rotation() drivers.Rotation {
	return drivers.Rotation0
}

func (d gbaDisplay) SetRotation(rotation drivers.Rotation) error {
	return errNoRotation
}

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
		e |= keyReleased
	}

	// This button event was read, so mark it as such.
	// By toggling the bit, the bit will be set to the value that is currently
	// in b.state.
	b.previousState ^= (1 << index)

	return e
}
