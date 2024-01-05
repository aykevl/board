//go:build pinetime

package board

import (
	"device/arm"
	"device/nrf"
	"machine"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/bma42x"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
)

const (
	Name = "pinetime"

	touchInterruptPin   = 28
	spiFlashCSPin       = machine.Pin(5)
	chargeIndicationPin = machine.Pin(12)
	powerPresencePin    = machine.Pin(19)
	batteryVoltagePin   = machine.Pin(31)
)

var (
	Power   = &mainBattery{}
	Sensors = allSensors{}
	Display = mainDisplay{}
	Buttons = &singleButton{}
)

func init() {
	// Enable the DC/DC regulator.
	// This doesn't affect sleep power consumption, but significantly reduces
	// runtime power consumpton of the CPU core (almost halving the current
	// required).
	nrf.POWER.DCDCEN.Set(nrf.POWER_DCDCEN_DCDCEN)

	// The UART is left enabled in the Wasp-OS bootloader.
	// This causes a 1.25mA increase in current consumption.
	// https://github.com/wasp-os/wasp-bootloader/pull/3
	nrf.UART0.ENABLE.Set(0)
}

type mainBattery struct {
	lastPercent int8
	chargePPM   int32
}

var batteryPercent = batteryApproximation{
	// Data is taken from this pull request:
	// https://github.com/InfiniTimeOrg/InfiniTime/pull/1444/files
	voltages: [6]uint16{3500, 3600, 3700, 3750, 3900, 4180},
	percents: [6]int8{0, 10, 25, 50, 75, 100},
}

func (b *mainBattery) Configure() {
	chargeIndicationPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	powerPresencePin.Configure(machine.PinConfig{Mode: machine.PinInput})

	// Configure the ADC.
	// Using just one sample (instead of 256 for example), because we have our
	// own filtering and long sample times actually drain a lot of power: around
	// 6µA when measuing the battery every 5 seconds.
	machine.InitADC()
	machine.ADC{Pin: batteryVoltagePin}.Configure(machine.ADCConfig{
		Reference:  3000,
		SampleTime: 40, // use the longest acquisition time
		Samples:    1,
	})
}

func (b *mainBattery) Status() (status ChargeState, microvolts uint32, percent int8) {
	rawValue := machine.ADC{Pin: batteryVoltagePin}.Get()
	// Formula to calculate microvolts:
	//   rawValue * 6000_000 / 0x10000
	// Simlified, to fit in 32-bit integers:
	//   rawValue * (6000_000/128) / (0x1000/128)
	//   rawValue * 46875 / 512
	microvolts = uint32(rawValue) * 46875 / 512
	isCharging := chargeIndicationPin.Get() == false  // low when charging
	isPowerPresent := powerPresencePin.Get() == false // low when present
	if isCharging {
		status = Charging
	} else if isPowerPresent {
		status = NotCharging
	} else {
		status = Discharging
	}

	// TODO: percent while charging
	percentPPM := batteryPercent.approximatePPM(microvolts)
	if b.chargePPM == 0 {
		// first measurement, probably
		b.chargePPM = percentPPM
	} else {
		b.chargePPM = (b.chargePPM*255 + percentPPM) / 256
	}
	newPercent := b.chargePPM / 10000
	if newPercent < int32(b.lastPercent) || newPercent > int32(b.lastPercent)+1 {
		// do some basic hysteresis
		b.lastPercent = int8(newPercent)
	}
	percent = b.lastPercent
	return
}

var spi0Configured bool

// Return SPI0 initialized and ready to use, configuring it if not already done.
func getSPI0() machine.SPI {
	spi := machine.SPI0
	if !spi0Configured {
		// Set the chip select line for the flash chip to inactive.
		spiFlashCSPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
		spiFlashCSPin.High()

		// Set the chip select line for the LCD controller to inactive.
		machine.LCD_CS.Configure(machine.PinConfig{Mode: machine.PinOutput})
		machine.LCD_CS.High()

		// Configure the SPI bus.
		spi.Configure(machine.SPIConfig{
			Frequency: 8_000_000, // 8MHz is the maximum the nrf52832 supports
			SCK:       machine.SPI0_SCK_PIN,
			SDO:       machine.SPI0_SDO_PIN,
			SDI:       machine.SPI0_SDI_PIN,
			Mode:      3,
		})

		// Put the flash controller in deep power-down.
		// This is done so that as long as the SPI flash isn't explicitly
		// initialized, it won't waste any power.
		spiFlashCSPin.Low()
		spi.Tx([]byte{0xB9}, nil) // deep power down
		spiFlashCSPin.High()
	}
	return spi
}

type mainDisplay struct{}

var display *st7789.DeviceOf[pixel.RGB444BE]

func (d mainDisplay) Configure() Displayer[pixel.RGB444BE] {
	// Configure the display.
	// RGB444 reduces theoretic update time by up to 25%, from 115.2ms to 86.4ms
	// (28.8ms reduction).
	spi := getSPI0()
	disp := st7789.NewOf[pixel.RGB444BE](spi,
		machine.LCD_RESET,
		machine.LCD_RS, // data/command
		machine.LCD_CS,
		machine.LCD_BACKLIGHT_HIGH) // TODO: allow better backlight control
	disp.Configure(st7789.Config{
		Width:      240,
		Height:     240,
		Rotation:   drivers.Rotation0,
		RowOffset:  80,
		FrameRate:  st7789.FRAMERATE_39,
		VSyncLines: 32, // needed for VBlank, not sure why
	})
	disp.EnableBacklight(true) // disable the backlight

	// Initialize these pins as regular pins too, for WaitForVBlank.
	machine.LCD_SCK.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LCD_SCK.Low()
	machine.LCD_SDI.Configure(machine.PinConfig{Mode: machine.PinOutput})

	display = &disp
	return display
}

func (d mainDisplay) MaxBrightness() int {
	return 1 // TODO: 0-7 is supported
}

func (d mainDisplay) SetBrightness(level int) {
	machine.LCD_BACKLIGHT_HIGH.Set(!(level > 0)) // low means on, high means off
}

func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	// Disable the SPI so we can manually communicate with the display.
	machine.SPI0.Bus.ENABLE.Set(nrf.SPIM_ENABLE_ENABLE_Disabled)

	// Wait until the scanline wraps around to 0.
	// This is also what the TE line does internally.
	// TODO: use time.Sleep() if we can, to save power.
	for readDisplayValue(st7789.GSCAN, 16) == 0 {
	}
	for readDisplayValue(st7789.GSCAN, 16) != 0 {
	}

	// Re-enable the SPI.
	machine.SPI0.Bus.ENABLE.Set(nrf.SPIM_ENABLE_ENABLE_Enabled)
}

// Wait for enough time between bitbanged high and low SPI pulses.
func delaySPIClock() {
	// 4 cycles, or 62.5ns.
	// Together with the store, it is 6 cycles or 93.75ns.
	arm.Asm("nop\nnop\nnop\nnop")
}

// Read a single value from the display, for example GSCAN, RDDID, etc.
// The bits parameter indicates the number of bits that will be received.
func readDisplayValue(cmd uint8, bits int) uint32 {
	const (
		cs  = machine.LCD_CS
		dc  = machine.LCD_RS
		sdi = machine.LCD_SDI
		sck = machine.LCD_SCK
	)

	// Initialize bitbanged SPI.
	delaySPIClock()
	cs.Low()
	dc.Low()
	sdi.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Clock out the command.
	for i := 0; i < 8; i++ {
		sdi.Set(cmd&0x80 != 0)
		delaySPIClock()
		sck.High()
		delaySPIClock()
		sck.Low()
		cmd <<= 1
	}
	delaySPIClock()

	// Dummy clock cycle (necessary for 24-bit and 32-bit read commands,
	// according to the datasheet).
	if bits >= 24 {
		sck.High()
		delaySPIClock()
		sck.Low()
		delaySPIClock()
	}

	// Read the result over SPI.
	sdi.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	dc.High()
	value := uint32(0)
	for i := 0; i < bits; i++ {
		sck.High()
		delaySPIClock()
		value <<= 1
		if sdi.Get() {
			value |= 1
		}
		sck.Low()
		delaySPIClock()
	}

	// Dummy clock cycle, according to the datasheet needed in all cases but in
	// my exprience only needed for 16-bit reads (GSCAN).
	if bits == 16 {
		sck.High()
		delaySPIClock()
		sck.Low()
		delaySPIClock()
	}

	// Finish the transaction.
	cs.High()
	dc.High()

	return value
}

func (d mainDisplay) PPI() int {
	return 261
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	// Configure touch interrupt pin.
	// After the pin goes low (for a very short time), the touch controller is
	// accessible over I2C for as long as a finger touches the screen and a
	// short time afterwards (a second or so) before going back to sleep.
	//
	// We don't actually use an interrupt here because pin change interrupts
	// result in far too much current consumption (jumping from 0.19mA to
	// 0.65mA), probably due to anomaly 97:
	// https://infocenter.nordicsemi.com/index.jsp?topic=%2Ferrata_nRF52832_Rev2%2FERR%2FnRF52832%2FRev2%2Flatest%2Fanomaly_832_97.html
	// Also see:
	// https://devzone.nordicsemi.com/f/nordic-q-a/50624/about-current-consumption-of-gpio-and-gpiote
	// We could use a PORT interrupt in GPIOTE, using it as a level interrupt.
	// And it would be a good idea to implement this in TinyGo directly (as a
	// level interrupt), but in the meantime we'll use this quick-n-dirty hack.
	nrf.P0.PIN_CNF[touchInterruptPin].Set(nrf.GPIO_PIN_CNF_DIR_Input<<nrf.GPIO_PIN_CNF_DIR_Pos | nrf.GPIO_PIN_CNF_INPUT_Connect<<nrf.GPIO_PIN_CNF_INPUT_Pos | nrf.GPIO_PIN_CNF_SENSE_Low<<nrf.GPIO_PIN_CNF_SENSE_Pos)

	configureI2CBus()

	return touchInput{}
}

var touchPoints [1]TouchPoint

type touchInput struct{}

var touchID uint32 = 1

var touchData = make([]byte, 6)

var touchInitialized bool

const touchI2CAddress = 0x15

func (input touchInput) ReadTouch() []TouchPoint {
	// The touch controller is very sparsely documented. You can find datasheet
	// in English and Chinese on the PineTime wiki:
	// https://wiki.pine64.org/wiki/PineTime#Component_Datasheets
	// The best documentation is in the Chinese documentation, you can use
	// Google Translate to translate it to English.

	// Read the bit from the LATCH reister, which is set to high when TP_INT
	// goes high but doesn't go low on its own. We do that manually once no more
	// touches are read from the touch controller.
	if nrf.P0.LATCH.Get()&(1<<touchInterruptPin) != 0 {
		if !touchInitialized {
			// Initialize the touch controller once we get the first touch.
			// Doing it this way as the I2C bus appears unresponsive outside a
			// touch event.
			touchInitialized = true

			// These are the values as set by InfiniTime.
			//     i2cBus.Tx(touchI2CAddress, []byte{0xEC, 0b00000101}, nil)
			//     i2cBus.Tx(touchI2CAddress, []byte{0xFA, 0b01110000}, nil)

			// MotionMask register:
			//   [0] EnDClick (disabled, enabled in InfiniTime)
			//   [1] EnConUD  (disabled)
			//   [2] EnConLR  (enabled)
			i2cBus.Tx(touchI2CAddress, []byte{0xEC, 0b0000_0100}, nil)

			// IrqCtl register:
			//   [7] EnTest   (disabled)
			//   [6] EnTouch  (enabled)
			//   [5] EnChange (enabled)
			//   [4] EnMotion (enabled)
			//   [0] OnceWLP  (disabled)
			i2cBus.Tx(touchI2CAddress, []byte{0xFA, 0b0111_0000}, nil)
		}

		i2cBus.ReadRegister(touchI2CAddress, 1, touchData)
		num := touchData[1] & 0x0f
		if num == 0 {
			touchID++ // for the next time
			// Stop reading touch events.
			// There may be a small race condition here, if the touch controller
			// detects another touch while reading the touch data over I2C.
			nrf.P0.LATCH.Set(1 << touchInterruptPin)
			touchPoints[0].ID = 0
			return nil
		}
		rawX := (uint16(touchData[2]&0xf) << 8) | uint16(touchData[3]) // x coord
		rawY := (uint16(touchData[4]&0xf) << 8) | uint16(touchData[5]) // y coord
		// Filter out erroneous data.
		if rawX >= 240 || rawY >= 240 {
			// X or Y are erroneous (this happens quite frequently).
			// Just return the previous value as a fallback.
			if touchPoints[0].ID != 0 {
				return touchPoints[:1]
			}
			return nil
		}
		x := int16(rawX)
		y := int16(rawY)
		if display != nil {
			// The screen is upside down from the configured rotation, so also
			// rotate the touch coordinates.
			if display.Rotation() == drivers.Rotation180 {
				x = 239 - x
				y = 239 - y
			}
		}
		touchPoints[0] = TouchPoint{
			X:  x,
			Y:  y,
			ID: touchID,
		}
		return touchPoints[:1]
	}
	return nil
}

// State for the one and only button on the PineTime.
type singleButton struct {
	state         bool
	previousState bool
}

func (b *singleButton) Configure() {
	// BUTTON_OUT must be held high for BUTTON_IN to read anything useful.
	machine.BUTTON_OUT.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BUTTON_OUT.Low()
	machine.BUTTON_IN.Configure(machine.PinConfig{Mode: machine.PinInput})
}

func (b *singleButton) ReadInput() {
	// BUTTON_OUT needs to be kept low most of the time to avoid a ~34µA current
	// increase. However, setting it to high just before reading doesn't appear
	// to be enough: a small delay is needed. This can be done by setting
	// BUTTON_OUT high multiple times in a row, which doesn't do anything except
	// introduce the needed delay.
	// Four stores appear to be enough to get readings, I have added a few more
	// for more reliable readings (especially as this is important for the
	// watchdog timer).
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.High()
	state := machine.BUTTON_IN.Get()
	machine.BUTTON_OUT.Low()
	b.state = state

	// Reset the watchdog timer only when the button is not pressed.
	// The watchdog is configured in the Wasp-OS bootloader, and we have to be
	// careful not to reset the watchdog while the button is pressed so that a
	// long press forces a WDT reset and lets us enter the bootloader.
	// For details, see:
	// https://wasp-os.readthedocs.io/en/latest/wasp.html#watchdog-protocol
	if !state {
		nrf.WDT.RR[0].Set(0x6E524635)
	}
}

func (b *singleButton) NextEvent() KeyEvent {
	if b.state == b.previousState {
		return NoKeyEvent
	}
	e := KeyEvent(KeyEnter)
	if !b.state {
		e |= keyReleased
	}
	b.previousState = b.state
	return e
}

var i2cBus *machine.I2C

func initI2CBus() {
	// Run I2C at a high speed (400KHz).
	i2cBus.Configure(machine.I2CConfig{
		Frequency: 400 * machine.KHz,
		SDA:       machine.Pin(6),
		SCL:       machine.Pin(7),
	})
}

func configureI2CBus() {
	if i2cBus == nil {
		i2cBus = machine.I2C1
		initI2CBus()

		// Disable the heart rate sensor on startup, to be enabled when a driver
		// configures it. It consumes around 110µA when left enabled.
		machine.I2C1.WriteRegister(0x44, 0x0C, []byte{0x00})
	}
}

type allSensors struct {
}

var accel *bma42x.Device

func (s allSensors) Configure(which drivers.Measurement) error {
	// Configure the accelerometer (either BMA421 or BMA425, depending on the
	// PineTime variant).
	accel = bma42x.NewI2C(machine.I2C1, bma42x.Address)
	err := accel.Configure(bma42x.Config{
		Device:   bma42x.DeviceBMA421 | bma42x.DeviceBMA425,
		Features: bma42x.FeatureStepCounting,
	})
	if err != nil {
		// Restart the I2C bus.
		// I don't know why, but configuring the BMA421 while it is already
		// configured freezes the I2C bus. The only recovery appears to be to
		// restart the I2C bus entirely.
		initI2CBus()
		err = accel.Configure(bma42x.Config{
			Device:   bma42x.DeviceBMA421 | bma42x.DeviceBMA425,
			Features: bma42x.FeatureStepCounting,
		})
	}
	return err
}

func (s allSensors) Update(which drivers.Measurement) error {
	if which&(drivers.Acceleration|drivers.Temperature) != 0 {
		err := accel.Update(which & (drivers.Acceleration | drivers.Temperature))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s allSensors) Acceleration() (x, y, z int32) {
	rawX, rawY, rawZ := accel.Acceleration()
	// Adjust accelerometer to match standard axes.
	x = -rawY
	y = -rawX
	z = -rawZ
	return
}

func (s allSensors) Steps() (steps uint32) {
	return accel.Steps()
}

func (s allSensors) Temperature() int32 {
	return accel.Temperature()
}
