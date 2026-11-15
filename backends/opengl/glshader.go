package opengl

import (
	"fmt"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gopxl/glhf/v2"
	"github.com/gopxl/mainthread/v2"
	"github.com/pkg/errors"

	"github.com/gopxl/pixel/v2"
)

// GLShader is a type to assist with managing a canvas's underlying
// shader configuration. This allows for customization of shaders on
// a per canvas basis.
type GLShader struct {
	s      *glhf.Shader
	vf, uf glhf.AttrFormat
	vs, fs string

	uniforms []gsUniformAttr

	// pages is whether the fragment shader samples through pixelTexture,
	// so it can draw a multi-page picture.
	pages bool

	uniformDefaults struct {
		transform mgl32.Mat3
		colormask mgl32.Vec4
		bounds    mgl32.Vec4
		texbounds mgl32.Vec4
		cliprect  mgl32.Vec4
	}

	// atlasPages is the value uAtlasPages holds in the compiled program.
	// It changes only when a paged picture is drawn, so it is set then and
	// not before every draw: on the web each uniform is a JS call.
	atlasPages float32
}

type gsUniformAttr struct {
	Name      string
	Type      glhf.AttrType
	value     interface{}
	ispointer bool

	isSampler bool
	tex       *glhf.Texture
	unit      int
	// used is whether the current fragment shader names the sampler. A
	// sampler left over from an earlier shader on the same canvas is not
	// bound.
	used bool

	// constant uniforms are set once when the shader is built, not before
	// every draw.
	constant bool
}

const (
	canvasPosition int = iota
	canvasColor
	canvasTexCoords
	canvasIntensity
	canvasClip
)

var defaultCanvasVertexFormat = glhf.AttrFormat{
	canvasPosition:  glhf.Attr{Name: "aPosition", Type: glhf.Vec2},
	canvasColor:     glhf.Attr{Name: "aColor", Type: glhf.Vec4},
	canvasTexCoords: glhf.Attr{Name: "aTexCoords", Type: glhf.Vec2},
	canvasIntensity: glhf.Attr{Name: "aIntensity", Type: glhf.Float},
	canvasClip:      glhf.Attr{Name: "aClipRect", Type: glhf.Vec4},
}

// NewGLShader sets up a base shader with everything needed for a Pixel
// canvas to render correctly. The defaults can be overridden
// by simply using the SetUniform function.
func NewGLShader(fragmentShader string) *GLShader {
	gs := &GLShader{
		vf: defaultCanvasVertexFormat,
		vs: BaseCanvasVertexShader,
		fs: fragmentShader,
	}

	gs.SetUniform("uTransform", &gs.uniformDefaults.transform)
	gs.SetUniform("uColorMask", &gs.uniformDefaults.colormask)
	gs.SetUniform("uBounds", &gs.uniformDefaults.bounds)
	gs.SetUniform("uTexBounds", &gs.uniformDefaults.texbounds)
	gs.uniforms = append(gs.uniforms, gsUniformAttr{
		Name:     "uAtlasPages",
		Type:     glhf.Float,
		value:    float32(0),
		constant: true,
	})
	for i := 1; i < pixel.PagedPictureMaxPages; i++ {
		gs.uniforms = append(gs.uniforms, gsUniformAttr{
			Name:     fmt.Sprintf("uAtlasPage%d", i),
			Type:     glhf.Int,
			value:    int32(atlasPageUnitBase + i - 1),
			constant: true,
		})
	}

	gs.Update()

	return gs
}

// Update reinitialize GLShader data and recompile the underlying gl shader object
func (gs *GLShader) Update() {
	gs.pages = strings.Contains(gs.fs, "pixelTexture(")
	for i := range gs.uniforms {
		if gs.uniforms[i].isSampler {
			gs.uniforms[i].used = strings.Contains(gs.fs, gs.uniforms[i].Name)
		}
	}
	gs.uf = make([]glhf.Attr, len(gs.uniforms))
	for idx := range gs.uniforms {
		gs.uf[idx] = glhf.Attr{
			Name: gs.uniforms[idx].Name,
			Type: gs.uniforms[idx].Type,
		}
	}

	var shader *glhf.Shader
	mainthread.Call(func() {
		var err error
		shader, err = glhf.NewShader(
			gs.vf,
			gs.uf,
			gs.vs,
			gs.fs,
		)
		if err != nil {
			panic(errors.Wrap(err, "failed to create Canvas, there's a bug in the shader"))
		}
		shader.Begin()
		for loc, u := range gs.uniforms {
			if u.constant {
				shader.SetUniformAttr(loc, u.Value())
			}
		}
		shader.End()
		gs.atlasPages = 0
	})

	gs.s = shader
}

// gets the uniform index from GLShader
func (gs *GLShader) getUniform(Name string) int {
	for i, u := range gs.uniforms {
		if u.Name == Name {
			return i
		}
	}
	return -1
}

// SetUniform appends a custom uniform name and value to the shader.
// if the uniform already exists, it will simply be overwritten.
//
// Example:
//
//	utime := float32(time.Since(starttime)).Seconds())
//	mycanvas.shader.AddUniform("u_time", &utime)
func (gs *GLShader) SetUniform(name string, value interface{}) {
	t, p := getAttrType(value)
	if loc := gs.getUniform(name); loc > -1 {
		gs.uniforms[loc].Name = name
		gs.uniforms[loc].Type = t
		gs.uniforms[loc].ispointer = p
		gs.uniforms[loc].value = value
		gs.uniforms[loc].isSampler = false
		return
	}
	gs.uniforms = append(gs.uniforms, gsUniformAttr{
		Name:      name,
		Type:      t,
		ispointer: p,
		value:     value,
	})
}

// Texture units SetUniformTexture may use. Unit 0 holds the picture being
// drawn (uTexture) and units from atlasPageUnitBase hold the pages of a paged
// picture, so a sampler on either would be overwritten before the draw.
const (
	MinUniformTextureUnit = 1
	MaxUniformTextureUnit = atlasPageUnitBase - 1
)

// SetUniformTexture makes tex available to the fragment shader as the
// sampler2D named name, on texture unit unit. unit must be between
// MinUniformTextureUnit and MaxUniformTextureUnit and distinct from any other
// sampler on the same shader; SetUniformTexture panics otherwise. tex is bound
// for each draw and unbound after it, with its own filtering and wrapping.
// Passing a nil tex leaves the sampler unbound.
func (gs *GLShader) SetUniformTexture(name string, tex *glhf.Texture, unit int) {
	if unit < MinUniformTextureUnit || unit > MaxUniformTextureUnit {
		panic(fmt.Sprintf("SetUniformTexture(%q): texture unit %d is reserved; use %d to %d (0 is the drawn picture, %d and up are atlas pages)",
			name, unit, MinUniformTextureUnit, MaxUniformTextureUnit, atlasPageUnitBase))
	}
	for _, u := range gs.uniforms {
		if u.isSampler && u.Name != name && u.unit == unit {
			panic(fmt.Sprintf("SetUniformTexture(%q): texture unit %d is already used by %q", name, unit, u.Name))
		}
	}
	unit32 := int32(unit)
	t, p := getAttrType(unit32)
	u := gsUniformAttr{
		Name:      name,
		Type:      t,
		value:     unit32,
		ispointer: p,
		isSampler: true,
		tex:       tex,
		unit:      unit,
		used:      strings.Contains(gs.fs, name),
	}
	if idx := gs.getUniform(name); idx > -1 {
		gs.uniforms[idx] = u
		return
	}
	gs.uniforms = append(gs.uniforms, u)
}

// bindSamplers binds every used sampler's texture to its unit and returns
// the function that unbinds them again. The shader must be bound; the active
// unit is 0 afterwards.
func (gs *GLShader) bindSamplers() func() {
	n := 0
	for _, u := range gs.uniforms {
		if u.isSampler && u.used && u.tex != nil {
			glhf.ActiveTexture(u.unit)
			u.tex.Begin()
			n++
		}
	}
	glhf.ActiveTexture(0)
	if n == 0 {
		return func() {}
	}
	return func() {
		for i := len(gs.uniforms) - 1; i >= 0; i-- {
			u := gs.uniforms[i]
			if u.isSampler && u.used && u.tex != nil {
				glhf.ActiveTexture(u.unit)
				u.tex.End()
			}
		}
		glhf.ActiveTexture(0)
	}
}

// Value returns the attribute's concrete value. If the stored value
// is a pointer, we return the dereferenced value.
func (gu *gsUniformAttr) Value() interface{} {
	if !gu.ispointer {
		return gu.value
	}
	switch gu.Type {
	case glhf.Vec2:
		return *gu.value.(*mgl32.Vec2)
	case glhf.Vec3:
		return *gu.value.(*mgl32.Vec3)
	case glhf.Vec4:
		return *gu.value.(*mgl32.Vec4)
	case glhf.Mat2:
		return *gu.value.(*mgl32.Mat2)
	case glhf.Mat23:
		return *gu.value.(*mgl32.Mat2x3)
	case glhf.Mat24:
		return *gu.value.(*mgl32.Mat2x4)
	case glhf.Mat3:
		return *gu.value.(*mgl32.Mat3)
	case glhf.Mat32:
		return *gu.value.(*mgl32.Mat3x2)
	case glhf.Mat34:
		return *gu.value.(*mgl32.Mat3x4)
	case glhf.Mat4:
		return *gu.value.(*mgl32.Mat4)
	case glhf.Mat42:
		return *gu.value.(*mgl32.Mat4x2)
	case glhf.Mat43:
		return *gu.value.(*mgl32.Mat4x3)
	case glhf.Int:
		return *gu.value.(*int32)
	case glhf.Float:
		return *gu.value.(*float32)
	default:
		panic("invalid attrtype")
	}
}

// Returns the type identifier for any (supported) attribute variable type
// and whether or not it is a pointer of that type.
func getAttrType(v interface{}) (glhf.AttrType, bool) {
	switch v.(type) {
	case int32:
		return glhf.Int, false
	case float32:
		return glhf.Float, false
	case mgl32.Vec2:
		return glhf.Vec2, false
	case mgl32.Vec3:
		return glhf.Vec3, false
	case mgl32.Vec4:
		return glhf.Vec4, false
	case mgl32.Mat2:
		return glhf.Mat2, false
	case mgl32.Mat2x3:
		return glhf.Mat23, false
	case mgl32.Mat2x4:
		return glhf.Mat24, false
	case mgl32.Mat3:
		return glhf.Mat3, false
	case mgl32.Mat3x2:
		return glhf.Mat32, false
	case mgl32.Mat3x4:
		return glhf.Mat34, false
	case mgl32.Mat4:
		return glhf.Mat4, false
	case mgl32.Mat4x2:
		return glhf.Mat42, false
	case mgl32.Mat4x3:
		return glhf.Mat43, false
	case *mgl32.Vec2:
		return glhf.Vec2, true
	case *mgl32.Vec3:
		return glhf.Vec3, true
	case *mgl32.Vec4:
		return glhf.Vec4, true
	case *mgl32.Mat2:
		return glhf.Mat2, true
	case *mgl32.Mat2x3:
		return glhf.Mat23, true
	case *mgl32.Mat2x4:
		return glhf.Mat24, true
	case *mgl32.Mat3:
		return glhf.Mat3, true
	case *mgl32.Mat3x2:
		return glhf.Mat32, true
	case *mgl32.Mat3x4:
		return glhf.Mat34, true
	case *mgl32.Mat4:
		return glhf.Mat4, true
	case *mgl32.Mat4x2:
		return glhf.Mat42, true
	case *mgl32.Mat4x3:
		return glhf.Mat43, true
	case *int32:
		return glhf.Int, true
	case *float32:
		return glhf.Float, true
	default:
		if v == nil {
			panic("invalid AttrType (nil)")
		}
		panic(fmt.Sprintf("invalid AttrType: %T value=%#v", v, v))
	}
}

var BaseCanvasVertexShader = `
#version 330 core

in vec2  aPosition;
in vec4  aColor;
in vec2  aTexCoords;
in float aIntensity;
in vec4  aClipRect;
in float aIsClipped;

out vec4  vColor;
out vec2  vTexCoords;
out float vIntensity;
out vec2  vPosition;
out vec4  vClipRect;

uniform mat3 uTransform;
uniform vec4 uBounds;

void main() {
	vec2 transPos = (uTransform * vec3(aPosition, 1.0)).xy;
	vec2 normPos = (transPos - uBounds.xy) / uBounds.zw * 2.0 - vec2(1.0, 1.0);
	gl_Position = vec4(normPos, 0.0, 1.0);

	vColor = aColor;
	vPosition = aPosition;
	vTexCoords = aTexCoords;
	vIntensity = aIntensity;
	vClipRect = aClipRect;
}
`

var BaseCanvasFragmentShader = `
#version 330 core

in vec4  vColor;
in vec2  vTexCoords;
in float vIntensity;
in vec4  vClipRect;

out vec4 fragColor;

uniform vec4 uColorMask;
uniform vec4 uTexBounds;
uniform sampler2D uTexture;
` + AtlasPagesGLSL + `
void main() {
	if ((vClipRect != vec4(0.0, 0.0, 0.0, 0.0)) && (gl_FragCoord.x < vClipRect.x || gl_FragCoord.y < vClipRect.y || gl_FragCoord.x > vClipRect.z || gl_FragCoord.y > vClipRect.w))
		discard;

	if (vIntensity == 0.0) {
		fragColor = uColorMask * vColor;
	} else {
		fragColor = vec4(0.0, 0.0, 0.0, 0.0);
		fragColor += (1.0 - vIntensity) * vColor;
		vec2 t = (vTexCoords - uTexBounds.xy) / uTexBounds.zw;
		fragColor += vIntensity * vColor * pixelTexture(t);
		fragColor *= uColorMask;
	}
}
`

// setAtlasPages sets uAtlasPages if it differs from what the program holds.
// The shader must be bound.
func (gs *GLShader) setAtlasPages(n float32) {
	if gs.atlasPages == n {
		return
	}
	if loc := gs.getUniform("uAtlasPages"); loc > -1 {
		gs.s.SetUniformAttr(loc, n)
	}
	gs.atlasPages = n
}
