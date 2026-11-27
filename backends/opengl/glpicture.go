package opengl

import (
	"math"
	"sync"
	"unsafe"

	"github.com/gopxl/glhf/v2"
	"github.com/gopxl/mainthread/v2"
	"github.com/gopxl/pixel/v2"
)

// GLPicture is a pixel.PictureColor with a Texture. All OpenGL Targets should implement and accept
// this interface, because it enables seamless drawing of one to another.
//
// Implementing this interface on an OpenGL Target enables other OpenGL Targets to efficiently draw
// that Target onto them.
type GLPicture interface {
	pixel.PictureColor
	Texture() *glhf.Texture
}

// pictureDataGLCache memoizes GLPicture uploads keyed by source *pixel.PictureData.
// A source shares both its CPU pixels and its texture across draw targets.
var pictureDataGLCache sync.Map // *pixel.PictureData -> GLPicture

// NewGLPicture creates a GLPicture with a static OpenGL texture, reusing the upload
// for a *pixel.PictureData source. Callers that update a picture must update both
// the source pixels and the associated texture.
//
// A *pixel.PagedPictureData gets one texture per page; see AtlasPagesGLSL.
func NewGLPicture(p pixel.Picture) GLPicture {
	if pp, ok := p.(*pixel.PagedPictureData); ok {
		return newGLPagedPicture(pp)
	}
	if pd, ok := p.(*pixel.PictureData); ok {
		if cached, found := pictureDataGLCache.Load(pd); found {
			return cached.(GLPicture)
		}
	}
	bounds := p.Bounds()
	bx, by, bw, bh := intBounds(bounds)

	gp := &glPicture{bounds: bounds}
	pd, _ := p.(*pixel.PictureData)
	var pixels []byte
	if pd != nil {
		gp.source = pd
		pixels = pictureDataUploadBytes(pd, bw, bh)
	} else {
		pixels = make([]byte, 4*bw*bh)
		gp.pixels = pixels
		if source, ok := p.(pixel.PictureColor); ok {
			for y := 0; y < bh; y++ {
				for x := 0; x < bw; x++ {
					at := pixel.V(
						math.Max(float64(bx+x), bounds.Min.X),
						math.Max(float64(by+y), bounds.Min.Y),
					)
					color := source.Color(at)
					off := (y*bw + x) * 4
					pixels[off+0] = uint8(color.R * 255)
					pixels[off+1] = uint8(color.G * 255)
					pixels[off+2] = uint8(color.B * 255)
					pixels[off+3] = uint8(color.A * 255)
				}
			}
		}
	}
	mainthread.Call(func() {
		gp.tex = glhf.NewTexture(bw, bh, false, pixels)
	})
	if pd != nil {
		pictureDataGLCache.Store(pd, gp)
	}
	return gp
}

// PictureData stores premultiplied RGBA8 in upload orientation. The backend
// consumes the bytes before NewTexture returns. Compact padded rows only when
// needed, and let Color keep using the source after the upload.
func pictureDataUploadBytes(pd *pixel.PictureData, width, height int) []byte {
	const pixelSize = int(unsafe.Sizeof(pd.Pix[0]))
	var _ [4]byte = [pixelSize]byte{}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(pd.Pix))), len(pd.Pix)*pixelSize)
	rowBytes := width * pixelSize
	if pd.Stride == width {
		return raw[:rowBytes*height]
	}
	pixels := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		start := y * pd.Stride * pixelSize
		copy(pixels[y*rowBytes:(y+1)*rowBytes], raw[start:start+rowBytes])
	}
	return pixels
}

type glPicture struct {
	bounds pixel.Rect
	tex    *glhf.Texture
	source *pixel.PictureData
	pixels []uint8
}

func (gp *glPicture) Bounds() pixel.Rect {
	return gp.bounds
}

func (gp *glPicture) Texture() *glhf.Texture {
	return gp.tex
}

func (gp *glPicture) Color(at pixel.Vec) pixel.RGBA {
	if gp.source != nil {
		return gp.source.Color(at)
	}
	if !gp.bounds.Contains(at) {
		return pixel.Alpha(0)
	}
	bx, by, bw, _ := intBounds(gp.bounds)
	x, y := int(at.X)-bx, int(at.Y)-by
	off := y*bw + x
	return pixel.RGBA{
		R: float64(gp.pixels[off*4+0]) / 255,
		G: float64(gp.pixels[off*4+1]) / 255,
		B: float64(gp.pixels[off*4+2]) / 255,
		A: float64(gp.pixels[off*4+3]) / 255,
	}
}
