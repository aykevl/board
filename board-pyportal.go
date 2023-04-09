//go:build pyportal

package board

import (
	"machine"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/ili9341"
	"tinygo.org/x/drivers/touch/resistive"
)

var (
	Display = mainDisplay{}
	Buttons = noButtons{}
)

type mainDisplay struct{}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	// Enable and configure display.
	display := ili9341.NewParallel(
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

	// Enable backlight.
	// TODO: do this in a separate method (and disable the backlight at
	// startup).
	backlight := machine.TFT_BACKLIGHT
	backlight.Configure(machine.PinConfig{Mode: machine.PinOutput})
	backlight.High()

	return display
}

func (d mainDisplay) Size() (width, height int16) {
	return 320, 240
}

func (d mainDisplay) PhysicalSize() (width, height int) {
	return 49, 37
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

func (input touchInput) ReadTouch() []TouchPoint {
	// Values calibrated on the PyPortal I have. Other boards might have
	// slightly different values.
	// TODO: make this configurable?
	const (
		xmin = 16000
		xmax = 54000
		ymax = 22000
		ymin = 48000
	)
	point := resistiveTouch.ReadTouchPoint()
	if point.Z > 8192 {
		if touchPoints[0].ID == 0 {
			touchID++
			touchPoints[0].ID = touchID
		}
		touchPoints[0].Y = int16(clamp(point.X, ymin, ymax, 0, 239))
		touchPoints[0].X = int16(clamp(point.Y, xmin, xmax, 0, 319))
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
