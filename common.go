package board

import (
	"time"
	"unsafe"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
)

var (
	AddressableLEDs LEDArray = dummyAddressableLEDs{}
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

	// How much time it takes (in nanoseconds) to draw a single pixel.
	// For example, for 8MHz and 16 bits per color:
	//     time.Second * 16 / 8e6
	WindowDrawSpeed time.Duration

	// Number of addressable LEDs used by default.
	AddressableLEDs int
}{
	WindowTitle:  "Simulator",
	WindowWidth:  240,
	WindowHeight: 240,
	WindowPPI:    120, // common on many modern displays (for example Retina is 254 / 2 = 127)

	// This matches common event badges like the PyBadge and the MCH2022 badge
	// (but not the SHA2017 badge which uses 6 RGBW LEDs).
	AddressableLEDs: 5,
}

// ChargeState is the charging status of a battery.
type ChargeState uint8

const (
	// A battery might be attached, this is unknown (no way to know by reading a
	// pin).
	UnknownBattery ChargeState = iota

	// This board doesn't have batteries.
	NoBattery

	// No battery is attached to the board, but there could be one.
	BatteryUnavailable

	// There is a battery attached and it's charging (usually from USB)
	Charging

	// Power is present, but the battery is not charging (usually when it is
	// fully charged).
	NotCharging

	// There is a battery attached and it's not charging (no power connected).
	Discharging
)

// Return a string representation of the charge status, mainly for debugging.
func (c ChargeState) String() string {
	switch c {
	default:
		return "unknown"
	case NoBattery:
		return "none"
	case BatteryUnavailable:
		return "not connected"
	case Charging:
		return "charging"
	case NotCharging:
		return "not charging"
	case Discharging:
		return "discharging"
	}
}

// A LED array is a sequence of individually addressable LEDs (like WS2812).
type LEDArray interface {
	// Configure the LED array. This needs to be called before any other method
	// (except Len).
	Configure()

	// Return the length of the LED array.
	Len() int

	// Set a given pixel to the RGB value. The index must be in bounds,
	// otherwise this method will panic. The value is not immediately visible,
	// call Update() to update the pixel array.
	// Note that LED arrays are usually indexed from the end, because of the way
	// data is sent to them.
	SetRGB(index int, r, g, b uint8)

	// Update the pixel array to the values previously set in SetRGB.
	Update()
}

// The display interface shared by all supported displays.
type Displayer[T pixel.Color] interface {
	// The display size in pixels.
	Size() (width, height int16)

	// DrawBitmap copies the bitmap to the internal buffer on the screen at the
	// given coordinates. It returns once the image data has been sent
	// completely.
	DrawBitmap(x, y int16, buf pixel.Image[T]) error

	// Display the written image on screen. This call may or may not be
	// necessary depending on the screen, but it's better to call it anyway.
	Display() error

	// Enter or exit sleep mode.
	Sleep(sleepEnabled bool) error

	// Return the current screen rotation.
	// Note that some screens are by default configured with rotation, so by
	// default you may not get drivers.Rotation0.
	Rotation() drivers.Rotation

	// Set a given rotation. For example, to rotate by 180° you can use:
	//
	// 	SetRotation((Rotation() + 2) % 4)
	//
	// Not all displays support rotation, in which case they will return an
	// error.
	SetRotation(drivers.Rotation) error
}

// TouchInput reads the touch screen (resistive/capacitive) on a display and
// returns the current list of touch points.
type TouchInput interface {
	ReadTouch() []TouchPoint
}

// A single touch point on the screen, from a finger, stylus, or something like
// that.
type TouchPoint struct {
	// ID for this touch point. New touch events get a monotonically
	// incrementing ID. Because it is a uint32 (and it's unlikely a screen will
	// be touched more than 4 billion times), it can be treated as a unique ID.
	ID uint32

	// X and Y pixel coordinates.
	X, Y int16
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

// Default lithium battery charge curve.
// This data is taken from the InfiniTime project:
// https://github.com/InfiniTimeOrg/InfiniTime/pull/1444
// It is unlikely to be very accurate for other batteries, but it's a reasonable
// approximation if no specific discharge curve has been made.
var lithumBatteryApproximation = batteryApproximation{
	voltages: [6]uint16{3500, 3600, 3700, 3750, 3900, 4180},
	percents: [6]int8{0, 10, 25, 50, 75, 100},
}

type batteryApproximation struct {
	voltages [6]uint16
	percents [6]int8
}

func (approx *batteryApproximation) approximate(microvolts uint32) int8 {
	if microvolts <= uint32(approx.voltages[0])*1000 {
		return 0 // below the lowest value
	}
	for i, v := range approx.voltages {
		if uint32(v)*1000 > microvolts {
			voltStart := uint32(approx.voltages[i-1]) * 1000
			voltEnd := uint32(v) * 1000
			percentStart := approx.percents[i-1]
			percentEnd := approx.percents[i]
			voltOffset := microvolts - voltStart
			voltDiff := voltEnd - voltStart
			percentDiff := percentEnd - percentStart
			percentOffset := voltOffset * uint32(percentDiff) / uint32(voltDiff)
			return int8(percentOffset + uint32(percentStart))
		}
	}
	// Outside the table, so must be 100%.
	return 100
}

func (approx *batteryApproximation) approximatePPM(microvolts uint32) int32 {
	if microvolts <= uint32(approx.voltages[0])*1000 {
		return 0 // below the lowest value
	}
	for i, v := range approx.voltages {
		if uint32(v)*1000 > microvolts {
			voltStart := uint32(approx.voltages[i-1]) // mV
			voltEnd := uint32(v)                      // mV
			percentStart := approx.percents[i-1]
			percentEnd := approx.percents[i]
			voltOffset := microvolts - voltStart*1000 // µV
			voltDiff := voltEnd - voltStart           // mV
			percentDiff := percentEnd - percentStart
			percentOffset := voltOffset * uint32(percentDiff) * 10 / voltDiff
			return int32(percentStart)*10000 + int32(percentOffset)
		}
	}
	// Outside the table, so must be 100%.
	return 1000_000
}

type dummyAddressableLEDs struct {
}

func (l dummyAddressableLEDs) Configure() {
	// Nothing to do here.
}

func (l dummyAddressableLEDs) Len() int {
	return 0 // always zero
}

func (l dummyAddressableLEDs) SetRGB(i int, r, g, b uint8) {
	panic("no LEDs on this board")
}

func (l dummyAddressableLEDs) Update() {
	// Nothing to do here.
}

type colorFormat interface {
	colorGRB
}

type colorGRB struct{ G, R, B uint8 }

// Convert pixel data to a byte slice, for sending it to WS2812 LEDs for
// example.
func pixelsToBytes[T colorFormat](pix []T) []byte {
	if len(pix) == 0 {
		return nil
	}
	var zeroColor T
	ptr := unsafe.Pointer(unsafe.SliceData(pix))
	return unsafe.Slice((*byte)(ptr), len(pix)*int(unsafe.Sizeof(zeroColor)))
}

// Dummy sensor value, to be embedded in actual drivers.Sensor implementations.
type baseSensors struct {
}

func (s baseSensors) Configure(which drivers.Measurement) error {
	return nil
}

func (s baseSensors) Update(which drivers.Measurement) error {
	return nil
}

func (s baseSensors) Acceleration() (x, y, z int32) {
	return 0, 0, 0
}

func (s baseSensors) Steps() uint32 {
	return 0
}

func (s baseSensors) Temperature() int32 {
	return 0
}
