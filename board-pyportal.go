//go:build pyportal

package board

import (
	"machine"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ili9341"
	"tinygo.org/x/drivers/touch/resistive"
)

const (
	Name = "pyportal"
)

var (
	Power           = dummyBattery{state: NoBattery}
	Sensors         = baseSensors{} // TODO: light, temperature
	Display         = mainDisplay{}
	Buttons         = noButtons{}
	AddressableLEDs = dummyAddressableLEDs{}
)

type mainDisplay struct{}

var display *ili9341.Device

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	// Initialize backlight and disable at startup.
	backlight := machine.TFT_BACKLIGHT
	backlight.Configure(machine.PinConfig{Mode: machine.PinOutput})
	backlight.Low()

	// Enable and configure display.
	display = ili9341.NewParallel(
		machine.LCD_DATA0,
		machine.TFT_WR,
		machine.TFT_DC,
		machine.TFT_CS,
		machine.TFT_RESET,
		machine.TFT_RD,
	)
	display.Configure(ili9341.Config{
		Rotation: ili9341.Rotation270,
	})

	// Enable the TE ("tearing effect") pin to read vblank status.
	te := machine.TFT_TE
	te.Configure(machine.PinConfig{Mode: machine.PinInput})
	display.EnableTEOutput(true)

	return display
}

func (d mainDisplay) MaxBrightness() int {
	return 1
}

func (d mainDisplay) SetBrightness(level int) {
	machine.TFT_BACKLIGHT.Set(level > 0)
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	// Wait until the display has finished updating.
	// TODO: wait for a pin interrupt instead of blocking.
	for machine.TFT_TE.Get() == true {
	}
	for machine.TFT_TE.Get() == false {
	}

}

func (d mainDisplay) PPI() int {
	return 166 // appears to be the same size/resolution as the Gopher Badge and the MCH2022 badge
}

// Configure the resistive touch input on this display.
func (d mainDisplay) ConfigureTouch() TouchInput {
	machine.InitADC()
	resistiveTouch.Configure(&resistive.FourWireConfig{
		YP: machine.TOUCH_YD,
		YM: machine.TOUCH_YU,
		XP: machine.TOUCH_XR,
		XM: machine.TOUCH_XL,
	})

	return touchInput{}
}

var resistiveTouch resistive.FourWire

var touchPoints [1]TouchPoint

type touchInput struct{}

var touchID uint32

// State associated with the touch input.
var (
	medianFilterX, medianFilterY medianFilter
	iirFilterX, iirFilterY       iirFilter
	lastPosX, lastPosY           int
)

func (input touchInput) ReadTouch() []TouchPoint {
	// Values calibrated on the PyPortal I have. Other boards might have
	// slightly different values.
	// TODO: make this configurable?
	const (
		xmin = 54000
		xmax = 16000
		ymin = 48000
		ymax = 22000
	)
	point := resistiveTouch.ReadTouchPoint()
	if point.Z > 8192 {
		medianFilterX.add(point.X)
		medianFilterY.add(point.Y)
		var posX, posY int
		if touchPoints[0].ID == 0 {
			// First touch on the touch screen.
			touchID++
			touchPoints[0].ID = touchID
			for i := 0; i < 4; i++ {
				// Initialize the median filter at this point with some more
				// samples, so that the entire median filter is filled.
				point := resistiveTouch.ReadTouchPoint()
				medianFilterX.add(point.X)
				medianFilterY.add(point.Y)
			}
			// Reset the IIR filter, and use the position as-is.
			iirFilterX.add(medianFilterX.value(), true)
			iirFilterY.add(medianFilterY.value(), true)
			posX = iirFilterX.value()
			posY = iirFilterY.value()
		} else {
			// New touch value while we were touching before.
			// Add the value to the IIR filter.
			iirFilterX.add(medianFilterX.value(), false)
			iirFilterY.add(medianFilterY.value(), false)
			// Use some hysteresis to avoid moving the point when it didn't
			// actually move.
			posX = lastPosX
			posY = lastPosY
			const diff = 400 // arbitrary value that appears to work well
			if iirFilterX.value() > lastPosX+diff {
				posX = iirFilterX.value() - diff
			}
			if iirFilterX.value() < lastPosX-diff {
				posX = iirFilterX.value() + diff
			}
			if iirFilterY.value() > lastPosY+diff {
				posY = iirFilterY.value() - diff
			}
			if iirFilterY.value() < lastPosY-diff {
				posY = iirFilterY.value() + diff
			}
		}
		lastPosX = posX
		lastPosY = posY
		x := int16(clamp(posX, ymin, ymax, 0, 239))
		y := int16(clamp(posY, xmin, xmax, 0, 319))
		if display != nil {
			// Adjust for screen rotation.
			switch display.Rotation() {
			case drivers.Rotation90:
				x, y = y, 239-x
			case drivers.Rotation180:
				x = 239 - x
				y = 319 - y
			case drivers.Rotation270:
				x, y = 319-y, x
			}
		}
		touchPoints[0].Y = y
		touchPoints[0].X = x
		return touchPoints[:1]
	} else {
		touchPoints[0].ID = 0
	}
	return nil
}

// Map and clamp an input value to an output range.
func clamp(value, lowIn, highIn, lowOut, highOut int) int {
	rangeIn := highIn - lowIn
	rangeOut := highOut - lowOut
	valueOut := (value - lowIn) * rangeOut / rangeIn
	if valueOut > highOut {
		valueOut = highOut
	}
	if valueOut < lowOut {
		valueOut = lowOut
	}
	return valueOut
}

// Touch screen filtering has been implemented using the description in this
// article:
// https://dlbeer.co.nz/articles/tsf.html
// It works a lot better than the rather naive algorithm I implemented before.

type medianFilter [5]int

func (f *medianFilter) add(n int) {
	// Shift the value into the array.
	f[0] = f[1]
	f[1] = f[2]
	f[2] = f[3]
	f[3] = f[4]
	f[4] = n
}

func (f *medianFilter) value() int {
	// Optimal sorting algorithm.
	// It is based on the sorting algorithm described here:
	// https://bertdobbelaere.github.io/sorting_networks.html
	sorted := *f
	compareSwap := func(a, b *int) {
		if *a > *b {
			*b, *a = *a, *b
		}
	}
	compareSwap(&sorted[1], &sorted[4])
	compareSwap(&sorted[0], &sorted[3])
	compareSwap(&sorted[1], &sorted[3])
	compareSwap(&sorted[0], &sorted[2])
	compareSwap(&sorted[2], &sorted[4])
	compareSwap(&sorted[0], &sorted[1])
	compareSwap(&sorted[1], &sorted[2])
	compareSwap(&sorted[3], &sorted[4])
	compareSwap(&sorted[2], &sorted[3])

	// Return the median value.
	return sorted[2]
}

// Infinite impulse response filter, to smooth the input values somewhat.
type iirFilter struct {
	state int
}

func (f *iirFilter) add(x int, reset bool) {
	if reset {
		f.state = x
	}
	// For every update, the new value is half of x and half of the old value,
	// added together:
	//   f.state = f.state*0.5 + x*0.5
	f.state = (f.state + x + 1) / 2
}

func (f *iirFilter) value() int {
	return f.state
}
