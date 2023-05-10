//go:build !baremetal

package board

// The generic board exists for testing locally without running on real
// hardware. This avoids potentially long edit-flash-test cycles.

import (
	"errors"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"github.com/veandco/go-sdl2/sdl"
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
	Power   = simulatedPower{}
	Display = mainDisplay{}
	Buttons = buttonsConfig{}
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

type sdlscreen struct {
	surface       *sdl.Surface // window surface
	framebuffer   *sdl.Surface // framebuffer as stored by DrawRGBBitmap8
	window        *sdl.Window
	scale         int
	brightness    bool
	keyevents     []KeyEvent
	keyeventsLock sync.Mutex
	touchID       uint32
	touches       [1]TouchPoint
	touchesLock   sync.Mutex
}

var screen = &sdlscreen{
	scale:      1,
	brightness: false,
}

var sdlStart sync.Once

func startSDL() {
	// Create a main loop for SDL2. I'm not entirely sure this is safe (it may
	// need to run on the main thread).
	mainRunning := make(chan struct{})
	sdlStart.Do(func() {
		go func() {
			runtime.LockOSThread()
			sdl.Main(func() {
				close(mainRunning)
				for {
					time.Sleep(time.Hour)
				}
			})
		}()
		<-mainRunning

		// Create the SDL window.
		sdl.Do(func() {
			var err error
			sdl.SetHint("SDL_VIDEODRIVER", "wayland,x11")
			sdl.Init(sdl.INIT_EVERYTHING)
			screen.window, err = sdl.CreateWindow(Simulator.WindowTitle, sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED, int32(Simulator.WindowWidth*screen.scale), int32(Simulator.WindowHeight*screen.scale), sdl.WINDOW_SHOWN|sdl.WINDOW_ALLOW_HIGHDPI)
			if err != nil {
				panic("failed to create SDL window: " + err.Error())
			}

			screen.surface, err = screen.window.GetSurface()
			if err != nil {
				panic("failed to create SDL surface: " + err.Error())
			}

			// Create framebuffer to write to.
			screen.framebuffer, err = sdl.CreateRGBSurfaceWithFormat(0, screen.surface.W, screen.surface.H, 8, screen.surface.Format.Format)
			if err != nil {
				panic("failed to create pseudo-framebuffer:" + err.Error())
			}

			// Fill framebuffer with random data. This simulates power-on behavior.
			var rect sdl.Rect
			for y := 0; y < int(screen.surface.H); y++ {
				for x := 0; x < int(screen.surface.W); x++ {
					c := rand.Uint32()
					rect.X = int32(x * screen.scale)
					rect.Y = int32(y * screen.scale)
					rect.W = int32(screen.scale)
					rect.H = int32(screen.scale)
					screen.framebuffer.FillRect(&rect, c)
				}
			}

			// Initialize display to black.
			screen.drawSurface()
			screen.window.UpdateSurface()
		})
	})
}

// Configure returns a new display ready to draw on.
//
// Boards without a display will return nil.
func (d mainDisplay) Configure() Displayer[pixel.RGB888] {
	// TODO: use something like golang.org/x/exp/shiny to avoid CGo.
	startSDL()
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
	screen.brightness = level > 0
	sdl.Do(func() {
		screen.drawSurface()
		screen.window.UpdateSurface()
	})
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

// Size of the display in pixels.
func (d mainDisplay) Size() (width, height int16) {
	return int16(Simulator.WindowWidth), int16(Simulator.WindowHeight)
}

// Pixels per inch for this display.
func (d mainDisplay) PPI() int {
	return Simulator.WindowPPI
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	startSDL()

	return sdltouch{}
}

func (s *sdlscreen) drawSurface() {
	if s.brightness {
		s.framebuffer.Blit(nil, s.surface, nil)
	} else {
		gray := sdl.MapRGB(s.framebuffer.Format, 96, 96, 96)
		s.surface.FillRect(nil, gray)
	}
}

func (s *sdlscreen) Display() error {
	sdl.Do(func() {
		s.drawSurface()
		for {
			event := sdl.PollEvent()
			if event == nil {
				break
			}
			switch event := event.(type) {
			case *sdl.QuitEvent:
				os.Exit(0)
			case *sdl.WindowEvent:
				s.window.UpdateSurface()
			case *sdl.KeyboardEvent:
				screen.keyeventsLock.Lock()
				keyevent := decodeSDLKeyboardEvent(event)
				screen.keyevents = append(screen.keyevents, keyevent)
				screen.keyeventsLock.Unlock()
			case *sdl.MouseButtonEvent:
				// Only capture left clicks with a mouse.
				if event.Button == sdl.BUTTON_LEFT {
					screen.touchesLock.Lock()
					switch event.Type {
					case sdl.MOUSEBUTTONDOWN:
						s.touchID++
						screen.touches[0] = TouchPoint{
							ID: s.touchID,
							X:  int16(event.X),
							Y:  int16(event.Y),
						}
					case sdl.MOUSEBUTTONUP:
						screen.touches[0] = TouchPoint{} // no active touch
					}
					screen.touchesLock.Unlock()
				}
			case *sdl.MouseMotionEvent:
				// Only capture dragging with the left mouse button.
				if event.Type == sdl.MOUSEMOTION && event.State&sdl.BUTTON_LEFT != 0 {
					screen.touchesLock.Lock()
					screen.touches[0] = TouchPoint{
						ID: s.touchID,
						X:  int16(event.X),
						Y:  int16(event.Y),
					}
					screen.touchesLock.Unlock()
				}
			}
		}
		screen.window.UpdateSurface()
	})
	return nil
}

func (s *sdlscreen) DrawRGBBitmap8(x, y int16, buf []byte, width, height int16) error {
	displayWidth, displayHeight := s.Size()
	if x < 0 || y < 0 || width <= 0 || height <= 0 ||
		x+width > displayWidth || y+height > displayHeight {
		return errors.New("board: drawing out of bounds")
	}
	var rect sdl.Rect
	drawStart := time.Now()
	lastUpdate := drawStart
	for bufy := 0; bufy < int(height); bufy++ {
		for bufx := 0; bufx < int(width); bufx++ {
			index := (bufy*int(width) + bufx) * 3
			c := sdl.MapRGB(s.framebuffer.Format, buf[index+0], buf[index+1], buf[index+2])
			rect.X = int32((bufx + int(x)) * s.scale)
			rect.Y = int32((bufy + int(y)) * s.scale)
			rect.W = int32(s.scale)
			rect.H = int32(s.scale)
			s.framebuffer.FillRect(&rect, c)
		}

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
				sdl.Do(func() {
					s.drawSurface()
					screen.window.UpdateSurface()
				})
				lastUpdate = now
			}
		}
	}
	return nil
}

func (s *sdlscreen) Size() (width, height int16) {
	bounds := s.surface.Bounds().Size()
	return int16(bounds.X / s.scale), int16(bounds.Y / s.scale)
}

// Set sleep mode for this screen.
func (s *sdlscreen) Sleep(sleepEnabled bool) error {
	// This is a no-op.
	// TODO: use a different gray than when the backlight is set to zero, to
	// indicate sleep mode.
	return nil
}

var errNoRotation = errors.New("error: SetRotation isn't supported")

func (s *sdlscreen) Rotation() drivers.Rotation {
	return drivers.Rotation0
}

func (s *sdlscreen) SetRotation(rotation drivers.Rotation) error {
	// TODO: implement this, to be able to test rotation support.
	return errNoRotation
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

func decodeSDLKeyboardEvent(event *sdl.KeyboardEvent) KeyEvent {
	var e KeyEvent
	switch event.Keysym.Sym {
	case sdl.K_LEFT:
		e = KeyLeft
	case sdl.K_RIGHT:
		e = KeyRight
	case sdl.K_UP:
		e = KeyUp
	case sdl.K_DOWN:
		e = KeyDown
	case sdl.K_ESCAPE:
		e = KeyEscape
	case sdl.K_RETURN:
		e = KeyEnter
	case sdl.K_SPACE:
		e = KeySpace
	case sdl.K_a:
		e = KeyA
	case sdl.K_b:
		e = KeyB
	default:
		return NoKeyEvent
	}
	if event.Type == sdl.KEYUP {
		e |= keyReleased
	}
	return e
}
