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

	"github.com/aykevl/tinygl/pixel"
	"tinygo.org/x/drivers"
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
	Power           = simulatedPower{}
	Display         = mainDisplay{}
	Buttons         = buttonsConfig{}
	AddressableLEDs = simulatedLEDs{}
)

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
	// Randomize the output a bit to fake ADC noise (programs should be able to
	// deal with that).
	microvolts = 3700_000 + rand.Uint32()%16384 - 8192
	return Discharging, microvolts, lithumBatteryApproximation.approximate(microvolts)
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

func (s *fyneScreen) DrawRGBBitmap8(x, y int16, buf []byte, width, height int16) error {
	displayWidth, displayHeight := s.Size()
	if x < 0 || y < 0 || width <= 0 || height <= 0 ||
		x+width > displayWidth || y+height > displayHeight {
		return errors.New("board: drawing out of bounds")
	}
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

type simulatedLEDs struct {
	Data []pixel.RGB888
}

// Initialize the addressable LEDs. This must be called once before writing data
// to the Data slice.
//
// The way to determine whether there are addressable LEDs on a given board, is
// to configure them and then check the length of board.AddressableLEDs.Data.
func (l *simulatedLEDs) Configure() {
	startWindow()
	l.Data = make([]pixel.RGB888, Simulator.AddressableLEDs)
	l.Update()
}

// Update the LEDs with the color data in the Data field.
//
// Data[0] typically refers to the last color in the array, not the first, due
// to the way these addressable LEDs are daisy-chained.
func (l *simulatedLEDs) Update() {
	cmd := fmt.Sprintf("addressable-leds %d", len(l.Data))
	windowSendCommand(cmd, pixelsToBytes(l.Data))
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
		default:
			fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		}
	}
}
