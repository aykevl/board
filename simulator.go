//go:build !baremetal

package board

// The simulator for a generic board. It can simulate various kinds of hardware,
// like:
//   * event badges
//   * GameBoy-like handhelds (Game Boy Advance, PyBadge)
//   * smartwatches
//   * boards with only a touchscreen (PyPortal).
// It is currently mostly made for boards with a display, but this is not at all
// a requirement in the API.
//
// The board API doesn't use a mainloop of any kind, which would not be
// necessary anyway on embedded systems. But it is necessary on OSes, so to work
// around this the simulator is actually run in a separate process by starting
// the current process again and communicating over pipes (stdin/stdout in the
// simulator process).

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/image/draw"
)

const runWindowCommand = "run-simulator-window"

func init() {
	if len(os.Args) >= 2 && os.Args[1] == runWindowCommand {
		// This is the simulator process.
		// Run the entire window in an init function, because that's the only
		// way to do this with the API that is exposed by the board package.
		windowMain()
		os.Exit(0)
	}
}

var (
	displayImageLock     sync.Mutex
	displayImage         *image.RGBA
	displayMaxBrightness = 1
	displayBrightness    = 0

	ledsLock   sync.Mutex
	leds       []color.RGBA
	ledsPerRow = 6
)

// The main function for the window process.
func windowMain() {
	// Create a raster image to use as a display buffer.
	displayImage = image.NewRGBA(image.Rect(0, 0, 240, 240))
	display := &displayWidget{}
	display.Generator = func(w, h int) image.Image {
		displayImageLock.Lock()
		defer displayImageLock.Unlock()
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(img, image.Rect(0, 0, w, h), image.NewUniform(color.RGBA{
			R: 192,
			G: 192,
			B: 192,
		}), image.Pt(0, 0), draw.Over)
		rect := displayImage.Bounds()
		scale := h / rect.Dy()
		width := rect.Dx() * scale
		height := rect.Dy() * scale
		x := (w - width) / 2
		y := (h - height) / 2
		displayRect := image.Rect(x, y, x+width, y+height)
		if displayBrightness <= 0 {
			// The backlight is off, so indicate this by making the screen gray.
			draw.Draw(img, displayRect, image.NewUniform(color.RGBA{
				R: 96,
				G: 96,
				B: 96,
			}), image.Pt(0, 0), draw.Src)
		} else {
			// Draw the display as usual.
			draw.NearestNeighbor.Scale(img, displayRect, displayImage, displayImage.Bounds(), draw.Src, nil)
		}
		return img
	}

	// Create LEDs.
	ledsWidget := canvas.NewRaster(func(w, h int) image.Image {
		ledsLock.Lock()
		defer ledsLock.Unlock()
		img := image.NewRGBA(image.Rect(0, 0, w, h))

		// Draw all the LEDs as squares, each 24 pixels in size with an 8 pixel
		// gap.
		rows := (len(leds) + ledsPerRow - 1) / ledsPerRow
		scale := float64(h) / float64(rows*32)
		col := 0
		row := 0
		for _, c := range leds {
			x0 := int(float64(8+col*32) * scale)
			x1 := int(float64(8+col*32+24) * scale)
			y0 := int(float64(row*32) * scale)
			y1 := int(float64(row*32+24) * scale)
			area := image.Rect(x0, y0, x1, y1)
			draw.Draw(img, area, image.NewUniform(c), image.Pt(0, 0), draw.Src)
			col++
			if col >= ledsPerRow {
				col = 0
				row++
			}
		}
		return img
	})
	ledsWidget.Hidden = true

	// Create a window.
	a := app.New()
	w := a.NewWindow("Simulator")
	w.SetPadded(false)
	w.SetFixedSize(true)
	w.SetContent(fyne.NewContainerWithLayout(layout.NewVBoxLayout(), display, ledsWidget))

	// Listen for keyboard events, and translate them to board API keycodes.
	if deskCanvas, ok := w.Canvas().(desktop.Canvas); ok {
		deskCanvas.SetOnKeyDown(func(event *fyne.KeyEvent) {
			key := decodeFyneKey(event.Name)
			if key != NoKey {
				fmt.Printf("keypress %d\n", key)
			}
		})
		deskCanvas.SetOnKeyUp(func(event *fyne.KeyEvent) {
			key := decodeFyneKey(event.Name)
			if key != NoKey {
				fmt.Printf("keyrelease %d\n", key)
			}
		})
	}

	// Listen for events from the parent process (which includes display data).
	go windowReceiveEvents(w, display, ledsWidget)

	// Show the window.
	w.ShowAndRun()
}

// Goroutine that listens for commands from the parent process.
func windowReceiveEvents(w fyne.Window, display *displayWidget, ledsWidget *canvas.Raster) {
	r := bufio.NewReader(os.Stdin)
	for {
		line, _ := r.ReadString('\n')
		cmd := strings.Fields(line)[0]
		switch cmd {
		case "display":
			var width, height int
			fmt.Sscanf(line, "%s %d %d\n", &cmd, &width, &height)
			newImage := image.NewRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					r := rand.Uint32()
					newImage.SetRGBA(x, y, color.RGBA{
						R: uint8(r >> 0),
						G: uint8(r >> 8),
						B: uint8(r >> 16),
					})
				}
			}

			displayImageLock.Lock()
			displayImage = newImage
			display.SetMinSize(fyne.NewSize(float32(width), float32(height)))
			displayImageLock.Unlock()
		case "display-brightness":
			displayImageLock.Lock()
			fmt.Sscanf(line, "%s %d %d\n", &cmd, &displayBrightness, displayMaxBrightness)
			displayImageLock.Unlock()
			display.Refresh()
		case "title":
			w.SetTitle(strings.TrimSpace(line[len("title"):]))
		case "draw":
			// Read the image data (which is a single line).
			var startX, startY, width int
			fmt.Sscanf(line, "%s %d %d %d\n", &cmd, &startX, &startY, &width)
			buf := make([]byte, width*3)
			io.ReadFull(r, buf)

			// Draw the image data to the image buffer.
			displayImageLock.Lock()
			for x := 0; x < width; x++ {
				displayImage.SetRGBA(startX+x, startY, color.RGBA{
					R: buf[x*3+0],
					G: buf[x*3+1],
					B: buf[x*3+2],
				})
			}
			displayImageLock.Unlock()
			display.Refresh()
		case "addressable-leds":
			// Read the LED data.
			var numLEDs int
			fmt.Sscanf(line, "%s %d\n", &cmd, &numLEDs)
			buf := make([]byte, numLEDs*3)
			io.ReadFull(r, buf)

			// Update the leds slice.
			ledsLock.Lock()
			if len(leds) != numLEDs {
				// LEDs were configured for the first time (probably).
				// Make sure we prepare for the given number of LEDs.
				leds = make([]color.RGBA, numLEDs)
				cols := ledsPerRow
				if cols > len(leds) {
					cols = len(leds)
				}
				rows := (len(leds) + ledsPerRow - 1) / ledsPerRow
				ledsWidget.SetMinSize(fyne.NewSize(float32(cols*32+8), float32(rows*32)))
				ledsWidget.Show()
			}
			for i := range leds {
				leds[len(leds)-i-1] = color.RGBA{
					R: buf[i*3+0],
					G: buf[i*3+1],
					B: buf[i*3+2],
				}
			}
			ledsLock.Unlock()
			ledsWidget.Refresh()
		default:
			fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		}
	}
}

func decodeFyneKey(key fyne.KeyName) KeyEvent {
	var e KeyEvent
	switch key {
	case fyne.KeyLeft:
		e = KeyLeft
	case fyne.KeyRight:
		e = KeyRight
	case fyne.KeyUp:
		e = KeyUp
	case fyne.KeyDown:
		e = KeyDown
	case fyne.KeyEscape:
		e = KeyEscape
	case fyne.KeyReturn:
		e = KeyEnter
	case fyne.KeySpace:
		e = KeySpace
	case fyne.KeyA:
		e = KeyA
	case fyne.KeyB:
		e = KeyB
	default:
		return NoKeyEvent
	}
	return e
}

var _ desktop.Mouseable = (*displayWidget)(nil)
var _ fyne.Draggable = (*displayWidget)(nil)

// Wrapper for canvas.Render that sends mouse events to the parent process.
type displayWidget struct {
	canvas.Raster
}

func (r *displayWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(&r.Raster)
}

func (r *displayWidget) MouseDown(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		fmt.Printf("mousedown %d %d\n", int(event.Position.X), int(event.Position.Y))
	}
}

func (r *displayWidget) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		fmt.Printf("mouseup\n")
	}
}

func (r *displayWidget) Dragged(event *fyne.DragEvent) {
	fmt.Printf("mousemove %d %d\n", int(event.PointEvent.Position.X), int(event.PointEvent.Position.Y))
}

func (r *displayWidget) DragEnd() {
	// handled in MouseUp
}
