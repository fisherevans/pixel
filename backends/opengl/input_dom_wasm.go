//go:build js && wasm

package opengl

import (
	"strings"
	"syscall/js"

	"github.com/gopxl/pixel/v2"
)

// domCodeToButton maps KeyboardEvent.code values to pixel.Button constants.
// Only keys the game consumes are mapped; unknown codes yield ok=false.
var domCodeToButton = map[string]pixel.Button{
	"Space":        pixel.KeySpace,
	"Quote":        pixel.KeyApostrophe,
	"Comma":        pixel.KeyComma,
	"Minus":        pixel.KeyMinus,
	"Period":       pixel.KeyPeriod,
	"Slash":        pixel.KeySlash,
	"Digit0":       pixel.Key0,
	"Digit1":       pixel.Key1,
	"Digit2":       pixel.Key2,
	"Digit3":       pixel.Key3,
	"Digit4":       pixel.Key4,
	"Digit5":       pixel.Key5,
	"Digit6":       pixel.Key6,
	"Digit7":       pixel.Key7,
	"Digit8":       pixel.Key8,
	"Digit9":       pixel.Key9,
	"Semicolon":    pixel.KeySemicolon,
	"Equal":        pixel.KeyEqual,
	"KeyA":         pixel.KeyA,
	"KeyB":         pixel.KeyB,
	"KeyC":         pixel.KeyC,
	"KeyD":         pixel.KeyD,
	"KeyE":         pixel.KeyE,
	"KeyF":         pixel.KeyF,
	"KeyG":         pixel.KeyG,
	"KeyH":         pixel.KeyH,
	"KeyI":         pixel.KeyI,
	"KeyJ":         pixel.KeyJ,
	"KeyK":         pixel.KeyK,
	"KeyL":         pixel.KeyL,
	"KeyM":         pixel.KeyM,
	"KeyN":         pixel.KeyN,
	"KeyO":         pixel.KeyO,
	"KeyP":         pixel.KeyP,
	"KeyQ":         pixel.KeyQ,
	"KeyR":         pixel.KeyR,
	"KeyS":         pixel.KeyS,
	"KeyT":         pixel.KeyT,
	"KeyU":         pixel.KeyU,
	"KeyV":         pixel.KeyV,
	"KeyW":         pixel.KeyW,
	"KeyX":         pixel.KeyX,
	"KeyY":         pixel.KeyY,
	"KeyZ":         pixel.KeyZ,
	"BracketLeft":  pixel.KeyLeftBracket,
	"Backslash":    pixel.KeyBackslash,
	"BracketRight": pixel.KeyRightBracket,
	"Backquote":    pixel.KeyGraveAccent,
	"Escape":       pixel.KeyEscape,
	"Enter":        pixel.KeyEnter,
	"Tab":          pixel.KeyTab,
	"Backspace":    pixel.KeyBackspace,
	"Insert":       pixel.KeyInsert,
	"Delete":       pixel.KeyDelete,
	"ArrowRight":   pixel.KeyRight,
	"ArrowLeft":    pixel.KeyLeft,
	"ArrowDown":    pixel.KeyDown,
	"ArrowUp":      pixel.KeyUp,
	"PageUp":       pixel.KeyPageUp,
	"PageDown":     pixel.KeyPageDown,
	"Home":         pixel.KeyHome,
	"End":          pixel.KeyEnd,
	"CapsLock":     pixel.KeyCapsLock,
	"ScrollLock":   pixel.KeyScrollLock,
	"NumLock":      pixel.KeyNumLock,
	"PrintScreen":  pixel.KeyPrintScreen,
	"Pause":        pixel.KeyPause,
	"F1":           pixel.KeyF1,
	"F2":           pixel.KeyF2,
	"F3":           pixel.KeyF3,
	"F4":           pixel.KeyF4,
	"F5":           pixel.KeyF5,
	"F6":           pixel.KeyF6,
	"F7":           pixel.KeyF7,
	"F8":           pixel.KeyF8,
	"F9":           pixel.KeyF9,
	"F10":          pixel.KeyF10,
	"F11":          pixel.KeyF11,
	"F12":          pixel.KeyF12,
	"Numpad0":      pixel.KeyKP0,
	"Numpad1":      pixel.KeyKP1,
	"Numpad2":      pixel.KeyKP2,
	"Numpad3":      pixel.KeyKP3,
	"Numpad4":      pixel.KeyKP4,
	"Numpad5":      pixel.KeyKP5,
	"Numpad6":      pixel.KeyKP6,
	"Numpad7":      pixel.KeyKP7,
	"Numpad8":      pixel.KeyKP8,
	"Numpad9":      pixel.KeyKP9,
	"NumpadDecimal":  pixel.KeyKPDecimal,
	"NumpadDivide":   pixel.KeyKPDivide,
	"NumpadMultiply": pixel.KeyKPMultiply,
	"NumpadSubtract": pixel.KeyKPSubtract,
	"NumpadAdd":      pixel.KeyKPAdd,
	"NumpadEnter":    pixel.KeyKPEnter,
	"NumpadEqual":    pixel.KeyKPEqual,
	"ShiftLeft":      pixel.KeyLeftShift,
	"ControlLeft":    pixel.KeyLeftControl,
	"AltLeft":        pixel.KeyLeftAlt,
	"MetaLeft":       pixel.KeyLeftSuper,
	"ShiftRight":     pixel.KeyRightShift,
	"ControlRight":   pixel.KeyRightControl,
	"AltRight":       pixel.KeyRightAlt,
	"MetaRight":      pixel.KeyRightSuper,
	"ContextMenu":    pixel.KeyMenu,
}

// initInput installs DOM keyboard listeners on the canvas. Events are
// translated into pixel.Button press/release/repeat on the shared
// InputHandler; printable characters are appended to the Typed buffer.
func (w *Window) initInput() {
	keyDown := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		code := ev.Get("code").String()
		btn, ok := domCodeToButton[code]
		if !ok {
			return nil
		}

		if ev.Get("repeat").Bool() {
			w.input.ButtonEvent(btn, pixel.Repeat)
		} else {
			w.input.ButtonEvent(btn, pixel.Press)
		}

		// Feed printable characters into Typed buffer. Browsers give us the
		// localized string in `event.key`; only single-code-point values
		// correspond to real characters (others are "Enter", "Shift", etc).
		key := ev.Get("key").String()
		if r, ok := singleRune(key); ok {
			w.input.CharEvent(r)
		}

		// Swallow default handling for game-consumed keys so the browser
		// doesn't scroll/tab-navigate while the canvas has focus.
		if shouldPreventDefault(code) {
			ev.Call("preventDefault")
		}
		return nil
	})

	keyUp := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		code := ev.Get("code").String()
		btn, ok := domCodeToButton[code]
		if !ok {
			return nil
		}
		w.input.ButtonEvent(btn, pixel.Release)
		if shouldPreventDefault(code) {
			ev.Call("preventDefault")
		}
		return nil
	})

	// Blur clears all held keys so we don't get stuck-down buttons when the
	// tab loses focus mid-keypress.
	blur := js.FuncOf(func(this js.Value, args []js.Value) any {
		for _, btn := range domCodeToButton {
			w.input.ButtonEvent(btn, pixel.Release)
		}
		return nil
	})

	w.jsCanvas.Call("addEventListener", "keydown", keyDown)
	w.jsCanvas.Call("addEventListener", "keyup", keyUp)
	w.jsCanvas.Call("addEventListener", "blur", blur)
}

func singleRune(s string) (rune, bool) {
	if s == "" {
		return 0, false
	}
	runes := []rune(s)
	if len(runes) != 1 {
		return 0, false
	}
	r := runes[0]
	if r < 0x20 || r == 0x7f {
		return 0, false
	}
	return r, true
}

// shouldPreventDefault reports whether the browser's default handling of a
// key should be suppressed while the canvas has focus.
func shouldPreventDefault(code string) bool {
	switch code {
	case "Space", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight",
		"Tab", "Backspace", "Slash", "Quote":
		return true
	}
	if strings.HasPrefix(code, "F") && len(code) > 1 {
		// Let F12 (devtools) through; block other F-keys.
		if code == "F12" {
			return false
		}
		return true
	}
	return false
}
