package opengl

import (
	"fmt"
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gopxl/glhf/v2"
	"github.com/gopxl/mainthread/v2"
	"github.com/pkg/errors"

	"github.com/gopxl/pixel/v2"
)

// Canvas is an off-screen rectangular BasicTarget and Picture at the same time, that you can draw
// onto.
//
// It supports TrianglesPosition, TrianglesColor, TrianglesPicture and PictureColor.
type Canvas struct {
	gf     *GLFrame
	shader *GLShader

	cmp    pixel.ComposeMethod
	mat    mgl32.Mat3
	col    mgl32.Vec4
	smooth bool

	sprite *pixel.Sprite
}

var _ pixel.ComposeTarget = (*Canvas)(nil)

// NewCanvas creates a new empty, fully transparent Canvas with given bounds.
func NewCanvas(bounds pixel.Rect) *Canvas {
	c := &Canvas{
		gf:  NewGLFrame(bounds),
		mat: mgl32.Ident3(),
		col: mgl32.Vec4{1, 1, 1, 1},
	}

	c.shader = NewGLShader(BaseCanvasFragmentShader)
	c.SetBounds(bounds)
	return c
}

// SetUniform will update the named uniform with the value of any supported underlying
// attribute variable. If the uniform already exists, including defaults, they will be reassigned
// to the new value. The value can be a pointer.
func (c *Canvas) SetUniform(name string, value interface{}) {
	c.shader.SetUniform(name, value)
}

// SetUniformTexture makes tex available to this canvas's fragment shader as
// the sampler2D uniform named name, on texture unit unit (1 to 8; 0 is the
// picture being drawn, uTexture). See GLShader.SetUniformTexture.
//
// The sampler is read at the same coordinates as the drawn picture only if
// the shader computes them so; a canvas texture of the same size as the
// drawn canvas, sampled at (vTexCoords - uTexBounds.xy) / uTexBounds.zw, lines
// up texel for texel.
func (c *Canvas) SetUniformTexture(name string, tex *glhf.Texture, unit int) {
	c.shader.SetUniformTexture(name, tex, unit)
}

// SetFragmentShader allows you to set a new fragment shader on the underlying
// framebuffer. Argument "src" is the GLSL source, not a filename.
func (c *Canvas) SetFragmentShader(src string) {
	c.shader.fs = src
	c.shader.Update()
}

// MakeTriangles creates a specialized copy of the supplied Triangles that draws onto this Canvas.
//
// TrianglesPosition, TrianglesColor and TrianglesPicture are supported.
func (c *Canvas) MakeTriangles(t pixel.Triangles) pixel.TargetTriangles {
	if gt, ok := t.(*GLTriangles); ok {
		return &canvasTriangles{
			GLTriangles: gt,
			dst:         c,
		}
	}
	return &canvasTriangles{
		GLTriangles: NewGLTriangles(c.shader, t),
		dst:         c,
	}
}

// MakePicture create a specialized copy of the supplied Picture that draws onto this Canvas.
//
// PictureColor is supported.
func (c *Canvas) MakePicture(p pixel.Picture) pixel.TargetPicture {
	if cp, ok := p.(*canvasPicture); ok {
		return &canvasPicture{
			GLPicture: cp.GLPicture,
			dst:       c,
		}
	}
	if gp, ok := p.(GLPicture); ok {
		return &canvasPicture{
			GLPicture: gp,
			dst:       c,
		}
	}
	return &canvasPicture{
		GLPicture: NewGLPicture(p),
		dst:       c,
	}
}

// SetMatrix sets a Matrix that every point will be projected by.
func (c *Canvas) SetMatrix(m pixel.Matrix) {
	// pixel.Matrix is 3x2 with an implicit 0, 0, 1 row after it. So
	// [0] [2] [4]    [0] [3] [6]
	// [1] [3] [5] => [1] [4] [7]
	//  0   0   1      0   0   1
	// since all matrix ops are affine, the last row never changes, and we don't need to copy it
	for i, j := range [...]int{0, 1, 3, 4, 6, 7} {
		c.mat[j] = float32(m[i])
	}
}

// SetColorMask sets a color that every color in triangles or a picture will be multiplied by.
func (c *Canvas) SetColorMask(col color.Color) {
	rgba := pixel.Alpha(1)
	if col != nil {
		rgba = pixel.ToRGBA(col)
	}
	c.col = mgl32.Vec4{
		float32(rgba.R),
		float32(rgba.G),
		float32(rgba.B),
		float32(rgba.A),
	}
}

// SetComposeMethod sets a Porter-Duff composition method to be used in the following draws onto
// this Canvas.
func (c *Canvas) SetComposeMethod(cmp pixel.ComposeMethod) {
	c.cmp = cmp
}

// SetBounds resizes the Canvas to the new bounds. Old content will be preserved.
func (c *Canvas) SetBounds(bounds pixel.Rect) {
	c.gf.SetBounds(bounds)
	if c.sprite == nil {
		c.sprite = pixel.NewSprite(nil, pixel.Rect{})
	}
	c.sprite.Set(c, c.Bounds())
	// c.sprite.SetMatrix(pixel.IM.Moved(c.Bounds().Center()))
}

// Bounds returns the rectangular bounds of the Canvas.
func (c *Canvas) Bounds() pixel.Rect {
	return c.gf.Bounds()
}

// SetSmooth sets whether stretched Pictures drawn onto this Canvas should be drawn smooth or
// pixely.
func (c *Canvas) SetSmooth(smooth bool) {
	c.smooth = smooth
}

// Smooth returns whether stretched Pictures drawn onto this Canvas are set to be drawn smooth or
// pixely.
func (c *Canvas) Smooth() bool {
	return c.smooth
}

// must be manually called inside mainthread
func (c *Canvas) setGlhfBounds() {
	_, _, bw, bh := intBounds(c.gf.Bounds())
	glhf.Bounds(0, 0, bw, bh)
}

// must be manually called inside mainthread
func setBlendFunc(cmp pixel.ComposeMethod) {
	switch cmp {
	case pixel.ComposeOver:
		glhf.BlendFunc(glhf.One, glhf.OneMinusSrcAlpha)
	case pixel.ComposeIn:
		glhf.BlendFunc(glhf.DstAlpha, glhf.Zero)
	case pixel.ComposeOut:
		glhf.BlendFunc(glhf.OneMinusDstAlpha, glhf.Zero)
	case pixel.ComposeAtop:
		glhf.BlendFunc(glhf.DstAlpha, glhf.OneMinusSrcAlpha)
	case pixel.ComposeRover:
		glhf.BlendFunc(glhf.OneMinusDstAlpha, glhf.One)
	case pixel.ComposeRin:
		glhf.BlendFunc(glhf.Zero, glhf.SrcAlpha)
	case pixel.ComposeRout:
		glhf.BlendFunc(glhf.Zero, glhf.OneMinusSrcAlpha)
	case pixel.ComposeRatop:
		glhf.BlendFunc(glhf.OneMinusDstAlpha, glhf.SrcAlpha)
	case pixel.ComposeXor:
		glhf.BlendFunc(glhf.OneMinusDstAlpha, glhf.OneMinusSrcAlpha)
	case pixel.ComposePlus:
		glhf.BlendFunc(glhf.One, glhf.One)
	case pixel.ComposeCopy:
		glhf.BlendFunc(glhf.One, glhf.Zero)
	case pixel.ComposeMultiply:
		// rgb = src*dst + dst*(1-srcA), alpha = dstA: the source darkens what
		// is there, in proportion to its coverage; a transparent source texel
		// leaves the destination alone (with Zero for dst it erased it).
		// Matches ComposeMethod.Compose wherever the destination is opaque;
		// over a transparent destination the source adds nothing, where
		// Compose would show it (no blend factor expresses that term).
		glhf.BlendFuncSeparate(glhf.DstColor, glhf.OneMinusSrcAlpha, glhf.Zero, glhf.One)
	case pixel.ComposeScreen:
		// rgb = src + dst*(1-src), alpha = srcA + dstA*(1-srcA): the
		// premultiplied screen formula, as ComposeMethod.Compose. Alpha used
		// to stay dstA, which left rgb > alpha over a transparent destination.
		glhf.BlendFuncSeparate(glhf.One, glhf.OneMinusSrcColor, glhf.One, glhf.OneMinusSrcAlpha)
	default:
		panic(errors.New("Canvas: invalid compose method"))
	}
}

// Clear fills the whole Canvas with a single color.
func (c *Canvas) Clear(color color.Color) {
	c.gf.Dirty()

	rgba := pixel.ToRGBA(color)

	// color masking
	rgba = rgba.Mul(pixel.RGBA{
		R: float64(c.col[0]),
		G: float64(c.col[1]),
		B: float64(c.col[2]),
		A: float64(c.col[3]),
	})

	// Use Call (blocking) not CallNonBlock: on WASM, CallNonBlock spawns a
	// goroutine, and every WebGL call is a goroutine scheduling point — the
	// deferred clear would race with in-flight draw calls and wipe content
	// mid-render.
	mainthread.Call(func() {
		c.setGlhfBounds()
		c.gf.Frame().Begin()
		glhf.Clear(
			float32(rgba.R),
			float32(rgba.G),
			float32(rgba.B),
			float32(rgba.A),
		)
		c.gf.Frame().End()
	})
}

// Color returns the color of the pixel over the given position inside the Canvas.
func (c *Canvas) Color(at pixel.Vec) pixel.RGBA {
	return c.gf.Color(at)
}

// Texture returns the underlying OpenGL Texture of this Canvas.
//
// Implements GLPicture interface.
func (c *Canvas) Texture() *glhf.Texture {
	return c.gf.Texture()
}

// Frame returns the underlying OpenGL Frame of this Canvas.
func (c *Canvas) Frame() *glhf.Frame {
	return c.gf.frame
}

// SetPixels replaces the content of the Canvas with the provided pixels. The provided slice must be
// an alpha-premultiplied RGBA sequence of correct length (4 * width * height).
func (c *Canvas) SetPixels(pixels []uint8) {
	c.gf.Dirty()

	mainthread.Call(func() {
		tex := c.Texture()
		tex.Begin()
		tex.SetPixels(0, 0, tex.Width(), tex.Height(), pixels)
		tex.End()
	})
}

// Pixels returns an alpha-premultiplied RGBA sequence of the content of the Canvas.
func (c *Canvas) Pixels() []uint8 {
	var pixels []uint8

	mainthread.Call(func() {
		tex := c.Texture()
		tex.Begin()
		pixels = tex.Pixels(0, 0, tex.Width(), tex.Height())
		tex.End()
	})

	return pixels
}

// Draw draws the content of the Canvas onto another Target, transformed by the given Matrix, just
// like if it was a Sprite containing the whole Canvas.
func (c *Canvas) Draw(t pixel.Target, matrix pixel.Matrix) {
	c.sprite.Draw(t, matrix)
}

// DrawColorMask draws the content of the Canvas onto another Target, transformed by the given
// Matrix and multiplied by the given mask, just like if it was a Sprite containing the whole Canvas.
//
// If the color mask is nil, a fully opaque white mask will be used causing no effect.
func (c *Canvas) DrawColorMask(t pixel.Target, matrix pixel.Matrix, mask color.Color) {
	c.sprite.DrawColorMask(t, matrix, mask)
}

type canvasTriangles struct {
	*GLTriangles
	dst *Canvas
}

func (ct *canvasTriangles) draw(tex *glhf.Texture, bounds pixel.Rect) {
	ct.drawPages(tex, nil, bounds)
}

// drawPages draws with tex on unit 0 and, for a paged picture, pages[1:] on
// the atlas page units; bounds is then one page.
func (ct *canvasTriangles) drawPages(tex *glhf.Texture, pages []*glhf.Texture, bounds pixel.Rect) {
	ct.dst.gf.Dirty()

	// save the current state vars to avoid race condition
	cmp := ct.dst.cmp
	smt := ct.dst.smooth
	mat := ct.dst.mat
	col := ct.dst.col

	// Use Call (blocking) not CallNonBlock: same race as Canvas.Clear — on
	// WASM every WebGL call is a goroutine scheduling point, so a non-blocking
	// draw can be preempted by window.Update (SwapBuffers) before it executes,
	// producing a black frame.
	mainthread.Call(func() {
		ct.dst.setGlhfBounds()
		setBlendFunc(cmp)

		frame := ct.dst.gf.Frame()
		shader := ct.shader.s

		frame.Begin()
		shader.Begin()

		ct.shader.uniformDefaults.transform = mat
		ct.shader.uniformDefaults.colormask = col
		dstBounds := ct.dst.Bounds()
		ct.shader.uniformDefaults.bounds = mgl32.Vec4{
			float32(dstBounds.Min.X),
			float32(dstBounds.Min.Y),
			float32(dstBounds.W()),
			float32(dstBounds.H()),
		}

		bx, by, bw, bh := intBounds(bounds)
		ct.shader.uniformDefaults.texbounds = mgl32.Vec4{
			float32(bx),
			float32(by),
			float32(bw),
			float32(bh),
		}

		ct.shader.setAtlasPages(float32(len(pages)))
		ct.shader.setAtlasSmooth(smt)

		for loc, u := range ct.shader.uniforms {
			if u.constant {
				continue
			}
			ct.shader.s.SetUniformAttr(loc, u.Value())
		}
		unbindSamplers := ct.shader.bindSamplers()

		if tex == nil {
			ct.vs.Begin()
			ct.vs.Draw()
			ct.vs.End()
		} else {
			for i := 1; i < len(pages); i++ {
				glhf.ActiveTexture(atlasPageUnitBase + i - 1)
				pages[i].Begin()
				if pages[i].Smooth() != smt {
					pages[i].SetSmooth(smt)
				}
			}
			glhf.ActiveTexture(0)
			tex.Begin()

			if tex.Smooth() != smt {
				tex.SetSmooth(smt)
			}

			ct.vs.Begin()
			ct.vs.Draw()
			ct.vs.End()

			tex.End()
			for i := len(pages) - 1; i >= 1; i-- {
				glhf.ActiveTexture(atlasPageUnitBase + i - 1)
				pages[i].End()
			}
			glhf.ActiveTexture(0)
		}

		unbindSamplers()
		shader.End()
		frame.End()
	})
}

func (ct *canvasTriangles) Draw() {
	ct.draw(nil, pixel.Rect{})
}

type canvasPicture struct {
	GLPicture
	dst *Canvas
}

func (cp *canvasPicture) Draw(t pixel.TargetTriangles) {
	ct := t.(*canvasTriangles)
	if cp.dst != ct.dst {
		panic(fmt.Errorf("(%T).Draw: TargetTriangles generated by different Canvas", cp))
	}
	if pp, ok := cp.GLPicture.(*glPagedPicture); ok {
		if len(pp.texs) > 1 && !ct.shader.pages {
			panic(fmt.Errorf("(%T).Draw: a multi-page picture needs a fragment shader that samples with pixelTexel or pixelTexture (see AtlasPagesGLSL)", cp))
		}
		size := pp.src.PageSize()
		ct.drawPages(pp.texs[0], pp.texs, pixel.R(0, 0, size.X, size.Y))
		return
	}
	ct.draw(cp.GLPicture.Texture(), cp.GLPicture.Bounds())
}
