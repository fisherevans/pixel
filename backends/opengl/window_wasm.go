//go:build js && wasm

package opengl

import (
	"image/color"
	"syscall/js"

	"github.com/gopxl/glhf/v2"
	"github.com/gopxl/pixel/v2"
	"github.com/gopxl/pixel/v2/backends/internal"
	"github.com/pkg/errors"
)

// CanvasElementID names the HTML canvas the WASM backend will attach to.
// Override before calling NewWindow to target a different element.
var CanvasElementID = "game"

// WindowConfig mirrors the desktop struct so call-sites compile unchanged.
// Most fields are no-ops under WASM; only Title and Bounds are honored.
type WindowConfig struct {
	Title                  string
	Icon                   []pixel.Picture
	Bounds                 pixel.Rect
	Position               pixel.Vec
	Monitor                *Monitor
	Resizable              bool
	Undecorated            bool
	NoIconify              bool
	AlwaysOnTop            bool
	TransparentFramebuffer bool
	VSync                  bool
	Maximized              bool
	Invisible              bool
	SamplesMSAA            int
	BoundsLimits           pixel.Rect
}

// Window wraps an HTML5 canvas plus a WebGL2 rendering context. The internal
// Canvas handles all drawing; Update blits it to the default framebuffer and
// yields to requestAnimationFrame.
type Window struct {
	bounds pixel.Rect
	canvas *Canvas

	jsCanvas js.Value
	gl       js.Value

	closed        bool
	vsync         bool
	cursorVisible bool

	input                      internal.InputHandler
	prevJoy, currJoy, tempJoy  internal.JoystickState

	buttonCallback       func(win *Window, button pixel.Button, action pixel.Action)
	charCallback         func(win *Window, r rune)
	mouseEnteredCallback func(win *Window, entered bool)
	mouseMovedCallback   func(win *Window, pos pixel.Vec)
	scrollCallback       func(win *Window, scroll pixel.Vec)
}

var currWin *Window

// NewWindow binds to the configured HTML canvas, creates a WebGL2 context,
// and initializes glhf. Returns an error if the canvas or context is missing.
func NewWindow(cfg WindowConfig) (*Window, error) {
	doc := js.Global().Get("document")
	jsCanvas := doc.Call("getElementById", CanvasElementID)
	if !jsCanvas.Truthy() {
		return nil, errors.Errorf("canvas #%s not found", CanvasElementID)
	}

	gl := jsCanvas.Call("getContext", "webgl2", map[string]any{
		"alpha":              false,
		"antialias":          cfg.SamplesMSAA > 0,
		"premultipliedAlpha": true,
		"preserveDrawingBuffer": false,
	})
	if !gl.Truthy() {
		return nil, errors.New("webgl2 not available")
	}

	_, _, w, h := intBounds(cfg.Bounds)
	jsCanvas.Set("width", w)
	jsCanvas.Set("height", h)

	if cfg.Title != "" {
		doc.Set("title", cfg.Title)
	}

	glhf.SetContext(gl)
	glhf.Init()

	win := &Window{
		bounds:        cfg.Bounds,
		jsCanvas:      jsCanvas,
		gl:            gl,
		vsync:         cfg.VSync,
		cursorVisible: true,
	}

	win.canvas = NewCanvas(cfg.Bounds)
	currWin = win

	// Ensure the canvas can receive keyboard focus inside iframes.
	if jsCanvas.Get("tabIndex").Int() < 0 {
		jsCanvas.Set("tabIndex", 0)
	}
	jsCanvas.Call("focus")

	win.initInput()
	win.installContextLostHandler()

	return win, nil
}

// installContextLostHandler logs and full-page-reloads on WebGL context loss.
// True in-place recovery is out of scope; reload is the simplest safe action.
func (w *Window) installContextLostHandler() {
	lost := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call("preventDefault")
		}
		js.Global().Get("console").Call("warn", "webglcontextlost — reloading")
		js.Global().Get("location").Call("reload")
		return nil
	})
	w.jsCanvas.Call("addEventListener", "webglcontextlost", lost)
}

// Destroy releases the WebGL context.
func (w *Window) Destroy() {
	w.closed = true
}

// Update blits the internal canvas to the default framebuffer and yields to
// the browser's requestAnimationFrame so the next frame can be scheduled.
func (w *Window) Update() {
	w.syncCanvasSize()
	w.SwapBuffers()
	w.UpdateInput()
	awaitAnimationFrame()
}

// syncCanvasSize updates the canvas backing store to match its current CSS size
// (times devicePixelRatio). Call before each frame so window resizes and
// fullscreen transitions propagate into window.Bounds() without a JS callback.
func (w *Window) syncCanvasSize() {
	cssW := w.jsCanvas.Get("clientWidth").Int()
	cssH := w.jsCanvas.Get("clientHeight").Int()
	if cssW <= 0 || cssH <= 0 {
		return
	}
	dpr := js.Global().Get("devicePixelRatio").Float()
	if dpr < 1 {
		dpr = 1
	}
	targetW := int(float64(cssW) * dpr)
	targetH := int(float64(cssH) * dpr)
	curW := w.jsCanvas.Get("width").Int()
	curH := w.jsCanvas.Get("height").Int()
	if curW == targetW && curH == targetH {
		return
	}
	w.jsCanvas.Set("width", targetW)
	w.jsCanvas.Set("height", targetH)
	w.bounds = pixel.R(0, 0, float64(targetW), float64(targetH))
	w.canvas.SetBounds(w.bounds)
}

// SwapBuffers copies the current canvas texture into the default framebuffer.
func (w *Window) SwapBuffers() {
	fbW, fbH := w.framebufferSize()
	glhf.Bounds(0, 0, fbW, fbH)
	glhf.Clear(0, 0, 0, 0)

	w.canvas.gf.Frame().Begin()
	w.canvas.gf.Frame().Blit(
		nil,
		0, 0, w.canvas.Texture().Width(), w.canvas.Texture().Height(),
		0, 0, fbW, fbH,
	)
	w.canvas.gf.Frame().End()
}

func (w *Window) framebufferSize() (int, int) {
	return w.jsCanvas.Get("width").Int(), w.jsCanvas.Get("height").Int()
}

// awaitAnimationFrame blocks the calling goroutine until the browser fires
// the next rAF callback. This is how the WASM backend yields time to the
// JS event loop once per simulated frame.
func awaitAnimationFrame() {
	ch := make(chan struct{}, 1)
	var cb js.Func
	cb = js.FuncOf(func(this js.Value, args []js.Value) any {
		cb.Release()
		ch <- struct{}{}
		return nil
	})
	js.Global().Call("requestAnimationFrame", cb)
	<-ch
}

// Closed reports whether the window has been explicitly closed. The browser
// tab closing terminates the Go runtime, so this only reflects SetClosed.
func (w *Window) Closed() bool              { return w.closed }
func (w *Window) SetClosed(closed bool)     { w.closed = closed }

func (w *Window) Bounds() pixel.Rect        { return w.bounds }
func (w *Window) SetBounds(bounds pixel.Rect) {
	w.bounds = bounds
	_, _, width, height := intBounds(bounds)
	w.jsCanvas.Set("width", width)
	w.jsCanvas.Set("height", height)
	w.canvas.SetBounds(bounds)
}
func (w *Window) SetBoundsLimits(pixel.Rect) {}

func (w *Window) SetPos(pixel.Vec)    {}
func (w *Window) GetPos() pixel.Vec   { return pixel.ZV }
func (w *Window) SetTitle(title string) {
	js.Global().Get("document").Set("title", title)
}

func (w *Window) Focused() bool {
	return js.Global().Get("document").Call("hasFocus").Bool()
}

func (w *Window) SetVSync(vsync bool) { w.vsync = vsync }
func (w *Window) VSync() bool         { return w.vsync }

func (w *Window) SetCursorVisible(visible bool) {
	w.cursorVisible = visible
	style := w.jsCanvas.Get("style")
	if visible {
		style.Set("cursor", "auto")
	} else {
		style.Set("cursor", "none")
	}
}
func (w *Window) SetCursorDisabled() {
	w.cursorVisible = false
	w.jsCanvas.Get("style").Set("cursor", "none")
}
func (w *Window) CursorVisible() bool { return w.cursorVisible }

func (w *Window) SetMonitor(*Monitor)      {}
func (w *Window) Monitor() *Monitor         { return nil }

// Note: must be called before any GL work. Kept for parity with desktop.
func (w *Window) begin() {
	if currWin != w {
		currWin = w
	}
}
func (w *Window) end() {}

// Drawing delegation -------------------------------------------------------

func (w *Window) MakeTriangles(t pixel.Triangles) pixel.TargetTriangles {
	return w.canvas.MakeTriangles(t)
}
func (w *Window) MakePicture(p pixel.Picture) pixel.TargetPicture {
	return w.canvas.MakePicture(p)
}
func (w *Window) SetMatrix(m pixel.Matrix)                 { w.canvas.SetMatrix(m) }
func (w *Window) SetColorMask(c color.Color)               { w.canvas.SetColorMask(c) }
func (w *Window) SetComposeMethod(cmp pixel.ComposeMethod) { w.canvas.SetComposeMethod(cmp) }
func (w *Window) SetSmooth(smooth bool)                    { w.canvas.SetSmooth(smooth) }
func (w *Window) Smooth() bool                             { return w.canvas.Smooth() }
func (w *Window) Clear(c color.Color)                      { w.canvas.Clear(c) }
func (w *Window) Color(at pixel.Vec) pixel.RGBA            { return w.canvas.Color(at) }
func (w *Window) Canvas() *Canvas                          { return w.canvas }

// Show/Hide/Focus are no-ops in a browser environment.
func (w *Window) Show()  {}
func (w *Window) Hide()  {}
func (w *Window) Focus() { w.jsCanvas.Call("focus") }

// Clipboard access goes through the browser's clipboard API when available.
// For now we return empty strings; writes are best-effort and ignored when
// the API is unavailable (e.g. insecure origin).
func (w *Window) ClipboardText() string        { return "" }
func (w *Window) SetClipboardText(text string) {}
func (w *Window) Clipboard() string            { return "" }
func (w *Window) SetClipboard(str string)      {}
