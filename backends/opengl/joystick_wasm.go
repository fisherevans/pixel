//go:build js && wasm

package opengl

import "github.com/gopxl/pixel/v2"

// Gamepads are out of scope for the WASM backend; all queries report
// "not connected".

func (w *Window) JoystickPresent(pixel.Joystick) bool                                 { return false }
func (w *Window) JoystickName(pixel.Joystick) string                                  { return "" }
func (w *Window) JoystickButtonCount(pixel.Joystick) int                              { return 0 }
func (w *Window) JoystickAxisCount(pixel.Joystick) int                                { return 0 }
func (w *Window) JoystickPressed(pixel.Joystick, pixel.GamepadButton) bool            { return false }
func (w *Window) JoystickJustPressed(pixel.Joystick, pixel.GamepadButton) bool        { return false }
func (w *Window) JoystickJustReleased(pixel.Joystick, pixel.GamepadButton) bool       { return false }
func (w *Window) JoystickAxis(pixel.Joystick, pixel.GamepadAxis) float64              { return 0 }
