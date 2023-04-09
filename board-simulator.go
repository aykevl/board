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

// Default devices.
var (
	Display = Display0
)

type display0Config struct{}

var Display0 display0Config

type sdlscreen struct {
	surface       *sdl.Surface
	window        *sdl.Window
	scale         int
	keyevents     []KeyEvent
	keyeventsLock sync.Mutex
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
	})
	<-mainRunning
}

func (d display0Config) Configure() Displayer[pixel.RGB888] {
	// TODO: use something like golang.org/x/exp/shiny to avoid CGo.

	startSDL()

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

	return screen
}

// Size of the board in pixels.
func (d display0Config) Size() (width, height int16) {
	return int16(Simulator.WindowWidth), int16(Simulator.WindowHeight)
}

// Physical size in millimeters.
func (d display0Config) PhysicalSize() (width, height int) {
	// numPixels / PPI * 25.4 (where 25.4 the number of millimeters in an inch)
	width = int(float32(Simulator.WindowWidth) / float32(Simulator.WindowPPI) * 25.4)
	height = int(float32(Simulator.WindowHeight) / float32(Simulator.WindowPPI) * 25.4)
	return
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

type buttonsConfig struct{}

var Buttons buttonsConfig

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
		e |= KeyReleased
	}
	return e
}
