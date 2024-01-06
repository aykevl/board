//go:build !baremetal

package board

// The generic board exists for testing locally without running on real
// hardware. This avoids potentially long edit-flash-test cycles.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
)

const (
	// The board name, as passed to TinyGo in the "-target" flag.
	// This is the special name "simulator" for the simulator.
	Name = "simulator"
)

// List of all devices.
//
// Support varies by board, but all boards have the following peripherals
// defined.
var (
	Power   = simulatedPower{}
	Sensors = &simulatedSensors{}
	Display = mainDisplay{}
	Buttons = buttonsConfig{}
)

func init() {
	AddressableLEDs = &simulatedLEDs{}
}

type simulatedPower struct{}

// Configure the battery status reader. This must be called before calling
// Status.
func (p simulatedPower) Configure() {
	// Nothing to do here.
}

// Status returns the current charge status (charging, discharging) and the
// current voltage of the battery in microvolts. If the voltage is 0 it means
// there is no battery present, any other value means a value was read from the
// battery (but there may or may not be a battery attached).
//
// The percent is a rough approximation of the state of charge of the battery.
// The value -1 means the state of charge is unknown.
// It is often inaccurate while charging. It may be best to just show "charging"
// instead of a specific percentage.
func (p simulatedPower) Status() (state ChargeState, microvolts uint32, percent int8) {
	// Pretend we're running on battery power and the battery is at 3.7V
	// (typical lipo voltage).
	actualMicrovolts := uint32(3700_000)
	// Randomize the output a bit to fake ADC noise (programs should be able to
	// deal with that).
	microvolts = actualMicrovolts + rand.Uint32()%16384 - 8192
	// Use a stable percent though, otherwise BLE battery level notifications
	// will fluctuate way too much.
	percent = lithumBatteryApproximation.approximate(actualMicrovolts)
	return Discharging, microvolts, percent
}

type mainDisplay struct{}

type fyneScreen struct {
	width         int
	height        int
	keyevents     []KeyEvent
	keyeventsLock sync.Mutex
	touchID       uint32
	touches       [1]TouchPoint
	touchesLock   sync.Mutex
}

var screen = &fyneScreen{}

// Configure returns a new display ready to draw on.
//
// Boards without a display will return nil.
func (d mainDisplay) Configure() Displayer[pixel.RGB888] {
	startWindow()
	screen.width = Simulator.WindowWidth
	screen.height = Simulator.WindowHeight
	windowSendCommand(fmt.Sprintf("display %d %d", screen.width, screen.height), nil)
	return screen
}

// MaxBrightness returns the maximum brightness value. A maximum brightness
// value of 0 means that this display doesn't support changing the brightness.
func (d mainDisplay) MaxBrightness() int {
	return 1
}

// SetBrightness sets brightness level of the display. It should be:
//
//	0 ≤ level ≤ MaxBrightness
//
// A value of 0 turns the backlight off entirely (but may leave the display
// running with nothing visible).
func (d mainDisplay) SetBrightness(level int) {
	// Send the current and max brightness levels.
	windowSendCommand(fmt.Sprintf("display-brightness %d %d", level, 1), nil)
}

// Wait until the next vertical blanking interval (vblank) interrupt is
// received. If the vblank interrupt is not available, it waits until the time
// since the previous call to WaitForVBlank is the default interval instead.
//
// The vertical blanking interval is the time between two screen refreshes. The
// vblank interrupt happens at the start of this interval, and indicates the
// period where the framebuffer is not being touched and can be updated without
// tearing.
//
// Don't use this method for timing, because vblank varies by hardware. Instead,
// use time.Now() to determine the current time and the amount of time since the
// last screen refresh.
//
// TODO: this is not a great API (it's blocking), it may change in the future.
func (d mainDisplay) WaitForVBlank(defaultInterval time.Duration) {
	// I'm sure there is some SDL2 API we could use here, but I couldn't find
	// one easily so just emulate it.
	dummyWaitForVBlank(defaultInterval)
}

// Pixels per inch for this display.
func (d mainDisplay) PPI() int {
	return Simulator.WindowPPI
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	startWindow()

	return sdltouch{}
}

func (s *fyneScreen) Display() error {
	// Nothing to do here.
	return nil
}

func (s *fyneScreen) DrawBitmap(x, y int16, image pixel.Image[pixel.RGB888]) error {
	displayWidth, displayHeight := s.Size()
	width, height := image.Size()
	if x < 0 || y < 0 || width <= 0 || height <= 0 ||
		int(x)+width > int(displayWidth) || int(y)+height > int(displayHeight) {
		return errors.New("board: drawing out of bounds")
	}
	buf := image.RawBuffer()
	drawStart := time.Now()
	lastUpdate := drawStart
	for bufy := 0; bufy < int(height); bufy++ {
		// Delay drawing a bit, to simulate a slow SPI bus.
		if Simulator.WindowDrawSpeed != 0 {
			now := time.Now()
			expected := drawStart.Add(Simulator.WindowDrawSpeed * time.Duration(bufy*int(width)))
			delay := expected.Sub(now)
			if delay > 0 {
				time.Sleep(delay)
				now = time.Now()
			}

			if now.Sub(lastUpdate) > 5*time.Millisecond {
				lastUpdate = now
			}
		}

		index := (bufy * int(width)) * 3
		lineBuf := buf[index : index+int(width)*3]
		windowSendCommand(fmt.Sprintf("draw %d %d %d", x, int(y)+bufy, width), lineBuf)
	}
	return nil
}

func (s *fyneScreen) Size() (width, height int16) {
	return int16(s.width), int16(s.height)
}

// Set sleep mode for this screen.
func (s *fyneScreen) Sleep(sleepEnabled bool) error {
	// This is a no-op.
	// TODO: use a different gray than when the backlight is set to zero, to
	// indicate sleep mode.
	return nil
}

var errNoRotation = errors.New("error: SetRotation isn't supported")

func (s *fyneScreen) Rotation() drivers.Rotation {
	return drivers.Rotation0
}

func (s *fyneScreen) SetRotation(rotation drivers.Rotation) error {
	// TODO: implement this, to be able to test rotation support.
	return errNoRotation
}

func (s *fyneScreen) SetScrollArea(topFixedArea, bottomFixedArea int16) {
	windowSendCommand(fmt.Sprintf("scroll-start %d %d", topFixedArea, bottomFixedArea), nil)
}

func (s *fyneScreen) SetScroll(line int16) {
	windowSendCommand(fmt.Sprintf("scroll %d", line), nil)
}

func (s *fyneScreen) StopScroll() {
	windowSendCommand(fmt.Sprintf("scroll-stop"), nil)
}

type sdltouch struct{}

func (s sdltouch) ReadTouch() []TouchPoint {
	screen.touchesLock.Lock()
	defer screen.touchesLock.Unlock()

	if screen.touches[0].ID != 0 {
		return screen.touches[:1]
	}
	return nil
}

type buttonsConfig struct{}

func (b buttonsConfig) Configure() {
}

func (b buttonsConfig) ReadInput() {
}

func (b buttonsConfig) NextEvent() KeyEvent {
	screen.keyeventsLock.Lock()
	defer screen.keyeventsLock.Unlock()

	if len(screen.keyevents) != 0 {
		event := screen.keyevents[0]
		copy(screen.keyevents, screen.keyevents[1:])
		screen.keyevents = screen.keyevents[:len(screen.keyevents)-1]
		return event
	}
	return NoKeyEvent
}

type simulatedSensors struct {
	configured  drivers.Measurement
	lock        sync.Mutex
	accelSource [3]float64
	stepsSource uint32
	accel       [3]int32
	steps       uint32
	temp        int32
}

// Configure configures all sensors as specified in the which parameter.
// If there is an error, none of the sensors can be relied upon to work.
func (s *simulatedSensors) Configure(which drivers.Measurement) error {
	s.configured = which
	return nil
}

// Update updates the sensor values as given in the which parameter.
// All sensors in the which parameter must have been configured before, or the
// behavior may be unpredictable.
func (s *simulatedSensors) Update(which drivers.Measurement) error {
	if which != s.configured&which {
		// This is a bug. Don't check it on each board, but do check it in the
		// simulator.
		panic("asked to update sensors that weren't configured")
	}

	if which&drivers.Acceleration != 0 {
		s.lock.Lock()
		// Add some noise to the accelerometer to make the values more
		// realistic.
		s.accel[0] = rand.Int31n(30_000) - 15_000 + int32(s.accelSource[0]*1000_000) // x
		s.accel[1] = rand.Int31n(30_000) - 15_000 + int32(s.accelSource[1]*1000_000) // y
		s.accel[2] = rand.Int31n(30_000) - 15_000 + int32(s.accelSource[2]*1000_000) // z
		s.steps = s.stepsSource
		s.lock.Unlock()
	}
	if which&drivers.Temperature != 0 {
		// Temperature around 20°C (with some jitter thrown in for a good
		// simulation).
		s.temp = 20000 + rand.Int31n(200) - 100
	}
	return nil
}

// Acceleration returns the last read acceleration in µg (micro-gravity). This
// includes gravity: when one of the axes is pointing straight to Earth and the
// sensor is not moving the returned value will be around 1000000 or -1000000.
//
// The accelerometer values match those used on Android. When the device is
// lying flat on a table, the Z axis is around 1g. When the device is rotated
// 90° upright, the Y axis is around 1g. When the device is then rotated 90° to
// the left (counter-clockwise), the X axis is around 1g.
//
// The simulator returns values as if the device is held upright like you'd hold
// a phone while taking a selfie.
func (s *simulatedSensors) Acceleration() (x, y, z int32) {
	return s.accel[0], s.accel[1], s.accel[2]
}

// Steps returns the number of steps since the step counter started.
// The uint32 value is assumed to be large enough for all practical use cases.
//
// The value can be incremented from the simulator.
func (s *simulatedSensors) Steps() (steps uint32) {
	return s.steps
}

// Temperature returns the temperature that was last read from the sensor.
// If there are multiple temperature sensors on a given board, the most accurate
// result will be returned.
//
// The simulator returns a fixed temperature, with some jitter to make it look
// more like a real-world sensor (no sensor is without noise).
func (s *simulatedSensors) Temperature() int32 {
	return s.temp
}

type simulatedLEDs struct {
	data []byte
}

// Initialize the addressable LEDs.
//
// The way to determine whether there are addressable LEDs on a given board, is
// to configure them and then check the length of board.AddressableLEDs.Data.
func (l *simulatedLEDs) Configure() {
	startWindow()
	l.data = make([]byte, Simulator.AddressableLEDs*3)
	l.Update()
}

func (l *simulatedLEDs) Len() int {
	return len(l.data) / 3
}

func (l *simulatedLEDs) SetRGB(i int, r, g, b uint8) {
	l.data[i*3+0] = r
	l.data[i*3+1] = g
	l.data[i*3+2] = b
}

// Update the LEDs with the color data.
func (l *simulatedLEDs) Update() {
	cmd := fmt.Sprintf("addressable-leds %d", l.Len())
	windowSendCommand(cmd, l.data)
}

var (
	fyneStart    sync.Once
	windowLock   sync.Mutex
	windowStdin  io.WriteCloser
	windowStdout io.ReadCloser
)

// Ensure the window is running in a separate process, starting it if necessary.
func startWindow() {
	// Create a main loop for Fyne.
	windowRunning := make(chan struct{})
	fyneStart.Do(func() {
		// Start the separate process that manages the window.
		go func() {
			cmd := exec.Command(os.Args[0], runWindowCommand)
			cmd.Stderr = os.Stderr
			windowStdin, _ = cmd.StdinPipe()
			windowStdout, _ = cmd.StdoutPipe()
			err := cmd.Start()
			if err != nil {
				fmt.Fprintln(os.Stdout, "could not start window process:", err)
				os.Exit(1)
			}
			close(windowRunning)
			err = cmd.Wait()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				os.Exit(1)
			}
			// The window was closed, so exit.
			os.Exit(0)
		}()
		<-windowRunning

		// Listen for events (keyboard/touch).
		go windowListenEvents()

		// Do some initialization.
		windowSendCommand("title "+Simulator.WindowTitle, nil)
	})
}

// Send a command to the separate process that manages the window.
// The command is a single line (without newline). The data part is optional
// binary data that can be sent with the command. The size of this binary data
// must be part of the textual command.
func windowSendCommand(command string, data []byte) {
	windowLock.Lock()
	defer windowLock.Unlock()

	windowStdin.Write([]byte(command + "\n"))
	windowStdin.Write(data)
}

// Goroutine that listens for window events like button and touch (keyboard and
// mouse).
func windowListenEvents() {
	r := bufio.NewReader(windowStdout)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, "failed to read I/O events from child process:", err)
		}
		cmd := strings.Fields(line)[0]
		switch cmd {
		case "keypress", "keyrelease":
			// Read the key code.
			var key KeyEvent
			fmt.Sscanf(line, "%s %d", &cmd, &key)
			if cmd == "keyrelease" {
				key |= keyReleased
			}

			// Add the key code to the
			screen.keyeventsLock.Lock()
			screen.keyevents = append(screen.keyevents, key)
			screen.keyeventsLock.Unlock()
		case "mousedown":
			// Read the event.
			var x, y int16
			fmt.Sscanf(line, "%s %d %d", &cmd, &x, &y)

			// Update the touch state.
			screen.touchesLock.Lock()
			screen.touchID++
			screen.touches[0] = TouchPoint{
				ID: screen.touchID,
				X:  x,
				Y:  y,
			}
			screen.touchesLock.Unlock()
		case "mouseup":
			// End the current touch.
			screen.touchesLock.Lock()
			screen.touches[0] = TouchPoint{} // no active touch
			screen.touchesLock.Unlock()
		case "mousemove":
			// Read the event.
			var x, y int16
			fmt.Sscanf(line, "%s %d %d", &cmd, &x, &y)

			// Update the touch state.
			screen.touchesLock.Lock()
			if screen.touches[0].ID != 0 {
				screen.touches[0].X = x
				screen.touches[0].Y = y
			}
			screen.touchesLock.Unlock()
		case "accel":
			var x, y, z float64
			fmt.Sscanf(line, "%s %f %f %f", &cmd, &x, &y, &z)
			Sensors.lock.Lock()
			Sensors.accelSource[0] = x
			Sensors.accelSource[1] = y
			Sensors.accelSource[2] = z
			Sensors.lock.Unlock()
		case "steps":
			var n uint32
			fmt.Sscanf(line, "%s %d %d", &cmd, &n)
			Sensors.lock.Lock()
			Sensors.stepsSource = n
			Sensors.lock.Unlock()
		default:
			fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		}
	}
}
