//go:build pyportal

package board

import (
	"machine"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/ili9341"
	"tinygo.org/x/drivers/touch/resistive"
)

const (
	Name = "pyportal"
)

var (
	Power           = dummyBattery{state: NoBattery}
	Display         = mainDisplay{}
	Buttons         = noButtons{}
	AddressableLEDs = dummyAddressableLEDs{}
)

type mainDisplay struct{}

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	// Initialize backlight and disable at startup.
	backlight := machine.TFT_BACKLIGHT
	backlight.Configure(machine.PinConfig{Mode: machine.PinOutput})
	backlight.Low()

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
