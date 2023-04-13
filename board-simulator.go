//go:build !baremetal

package board

// The generic board exists for testing locally without running on real
// hardware. This avoids potentially long edit-flash-test cycles.

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aykevl/tinygl/pixel"
	"github.com/veandco/go-sdl2/sdl"
)

// List of all devices.
//
// Support varies by board, but all boards have the following peripherals
// defined.
var (
	Display = mainDisplay{}
	Buttons = buttonsConfig{}
)

type mainDisplay struct{}

type sdlscreen struct {
	surface       *sdl.Surface
	window        *sdl.Window
	scale         int
	keyevents     []KeyEvent
	keyeventsLock sync.Mutex
	touchID       uint32
	touches       [1]TouchPoint
	touchesLock   sync.Mutex
}

var screen = &sdlscreen{scale: 1}

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

			// Initialize display to black.
			screen.surface.FillRect(nil, 0)
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

// Physical size in millimeters.
func (d mainDisplay) PhysicalSize() (width, height int) {
	// numPixels / PPI * 25.4 (where 25.4 the number of millimeters in an inch)
	width = int(float32(Simulator.WindowWidth) / float32(Simulator.WindowPPI) * 25.4)
	height = int(float32(Simulator.WindowHeight) / float32(Simulator.WindowPPI) * 25.4)
	return
}

func (d mainDisplay) ConfigureTouch() TouchInput {
	startSDL()

	return sdltouch{}
}

func (s *sdlscreen) Display() error {
	sdl.Do(func() {
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
	var rect sdl.Rect
	for bufy := 0; bufy < int(height); bufy++ {
		for bufx := 0; bufx < int(width); bufx++ {
			index := (bufy*int(width) + bufx) * 3
			c := sdl.MapRGB(s.surface.Format, buf[index+0], buf[index+1], buf[index+2])
			rect.X = int32((bufx + int(x)) * s.scale)
			rect.Y = int32((bufy + int(y)) * s.scale)
			rect.W = int32(s.scale)
			rect.H = int32(s.scale)
			s.surface.FillRect(&rect, c)
		}
	}
	return nil
}

func (s *sdlscreen) Size() (width, height int16) {
	bounds := s.surface.Bounds().Size()
	return int16(bounds.X / s.scale), int16(bounds.Y / s.scale)
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
