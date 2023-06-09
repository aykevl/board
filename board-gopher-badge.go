//go:build gopher_badge

package board

import (
	"machine"
	"math/bits"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/drivers/ws2812"
)

const (
	Name = "gopher-badge"
)

var (
	Power           = dummyBattery{state: UnknownBattery}
	Display         = mainDisplay{}
	Buttons         = &gpioButtons{}
	AddressableLEDs = ws2812LEDs{Data: make([]pixel.LinearGRB888, 2)}
)

type mainDisplay struct{}

var display st7789.Device

func (d mainDisplay) Configure() Displayer[pixel.RGB565BE] {
	machine.SPI0.Configure(machine.SPIConfig{
		// Mode 3 appears to be compatible with mode 0, but is slightly
		// faster: each byte takes 9 clock cycles instead of 10.
		// TODO: try to eliminate this last bit? Two ideas:
		//   - use 16-bit transfers, to halve the time the gap takes
		//   - use PIO, which apparently is able to send data without gap
		// It would seem like TI mode would be faster (it has no gap), but
		// it samples data on the falling edge instead of on the rising edge
		// like the st7789 expects.
		Mode:      3,
		SCK:       machine.SPI0_SCK_PIN,
		SDO:       machine.SPI0_SDO_PIN,
		SDI:       machine.SPI0_SDI_PIN,
		Frequency: 62_500_000, // datasheet for st7789 says 16ns (62.5MHz) is the max clock speed
	})

	display = st7789.New(machine.SPI0,
		machine.TFT_RST,       // TFT_RESET
		machine.TFT_WRX,       // TFT_DC
		machine.TFT_CS,        // TFT_CS
		machine.TFT_BACKLIGHT) // TFT_LITE

	display.Configure(st7789.Config{
		Rotation: st7789.ROTATION_90,
		Height:   320,

		// Gamma data obtained from example code provided with the display:
		// https://www.buydisplay.com/2-4-inch-ips-240x320-tft-lcd-display-capacitive-touch-screen
		// Without these values, most colors (especially green) don't look right.
		PVGAMCTRL: []byte{0xF0, 0x00, 0x04, 0x04, 0x04, 0x05, 0x29, 0x33, 0x3E, 0x38, 0x12, 0x12, 0x28, 0x30},
		NVGAMCTRL: []byte{0xF0, 0x07, 0x0A, 0x0D, 0x0B, 0x07, 0x28, 0x33, 0x3E, 0x36, 0x14, 0x14, 0x29, 0x32},
	})
	display.EnableBacklight(false)

	return &display
}

func (d mainDisplay) MaxBrightness() int {
	return 1
}

func (d mainDisplay) SetBrightness(level int) {
	machine.TFT_BACKLIGHT.Set(level > 0)
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	// Lower the SPI frequency for reading: the ST7789 supports high frequency
	// writes but reading is much slower.
	machine.SPI0.SetBaudRate(10_000_000)

	// Wait until the scanline wraps around to 0.
	// This is also what the TE line does internally.
	for display.GetScanLine() == 0 {
	}
	for display.GetScanLine() != 0 {
	}

	// Restore old baud rate.
	machine.SPI0.SetBaudRate(62_500_000)
}

func (d mainDisplay) PPI() int {
	return 166 // 320px / (48.96mm / 25.4)
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
	machine.BUTTON_UP.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_LEFT.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_DOWN.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.BUTTON_RIGHT.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *gpioButtons) ReadInput() {
	state := uint8(0)
	if !machine.BUTTON_A.Get() {
		state |= 1
	}
	if !machine.BUTTON_B.Get() {
		state |= 2
	}
	if !machine.BUTTON_UP.Get() {
		state |= 4
	}
	if !machine.BUTTON_LEFT.Get() {
		state |= 8
	}
	if !machine.BUTTON_DOWN.Get() {
		state |= 16
	}
	if !machine.BUTTON_RIGHT.Get() {
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

type ws2812LEDs struct {
	Data []pixel.LinearGRB888
}

func (l *ws2812LEDs) Configure() {
	machine.WS2812.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

// Send pixel data to the LEDs.
func (l *ws2812LEDs) Update() {
	ws := ws2812.Device{Pin: machine.WS2812}
	ws.Write(pixelsToBytes(l.Data))
}
