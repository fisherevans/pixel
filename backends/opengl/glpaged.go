package opengl

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gopxl/glhf/v2"

	"github.com/gopxl/pixel/v2"
)

// Paged pictures (pixel.PagedPictureData) are drawn with every page bound at
// once: page 0 on texture unit 0 as uTexture, page i on unit
// atlasPageUnitBase+i-1 as uAtlasPage<i>. Units 1 to 8 stay free for a
// canvas's own samplers (SetUniformTexture). WebGL2 guarantees 16 fragment
// texture units, so this fits every target.
const atlasPageUnitBase = 9

// AtlasPagesGLSL declares the page samplers and pixelTexture(t), which
// samples the drawn picture at t = (vTexCoords - uTexBounds.xy) /
// uTexBounds.zw. For an ordinary picture that is texture(uTexture, t). For a
// paged picture uTexBounds.zw is one page, so the whole part of t names the
// page and the fraction is the position in it. A fragment shader that may
// draw a paged picture calls pixelTexture instead of texture(uTexture, ...);
// WithAtlasPages adds this source to one.
const AtlasPagesGLSL = `
uniform float uAtlasPages;
uniform sampler2D uAtlasPage1;
uniform sampler2D uAtlasPage2;
uniform sampler2D uAtlasPage3;
uniform sampler2D uAtlasPage4;
uniform sampler2D uAtlasPage5;
uniform sampler2D uAtlasPage6;
uniform sampler2D uAtlasPage7;

vec4 pixelTexture(vec2 t) {
	if (uAtlasPages < 0.5) {
		return texture(uTexture, t);
	}
	vec2 cell = floor(t);
	vec2 at = t - cell;
	int page = clamp(int(cell.x) + 4 * int(cell.y), 0, int(uAtlasPages) - 1);
	// textureLod: the branch is not uniform, so implicit derivatives are
	// undefined, and atlas pages have no mipmaps anyway.
	if (page == 0) return textureLod(uTexture, at, 0.0);
	if (page == 1) return textureLod(uAtlasPage1, at, 0.0);
	if (page == 2) return textureLod(uAtlasPage2, at, 0.0);
	if (page == 3) return textureLod(uAtlasPage3, at, 0.0);
	if (page == 4) return textureLod(uAtlasPage4, at, 0.0);
	if (page == 5) return textureLod(uAtlasPage5, at, 0.0);
	if (page == 6) return textureLod(uAtlasPage6, at, 0.0);
	return textureLod(uAtlasPage7, at, 0.0);
}
`

// WithAtlasPages returns a fragment shader source with AtlasPagesGLSL
// inserted after its uTexture declaration, so it can call pixelTexture. It
// panics if src does not declare "uniform sampler2D uTexture;".
func WithAtlasPages(src string) string {
	const decl = "uniform sampler2D uTexture;"
	i := strings.Index(src, decl)
	if i < 0 {
		panic(fmt.Errorf("WithAtlasPages: shader does not declare %q", decl))
	}
	i += len(decl)
	return src[:i] + "\n" + AtlasPagesGLSL + src[i:]
}

func init() {
	if pixel.PagedPictureColumns != 4 || pixel.PagedPictureMaxPages != 8 {
		panic("opengl: AtlasPagesGLSL assumes 4 columns and 8 pages")
	}
}

// glPagedPicture is the GLPicture of a pixel.PagedPictureData: one texture
// per page. Texture returns page 0.
type glPagedPicture struct {
	src   *pixel.PagedPictureData
	pages []GLPicture
	texs  []*glhf.Texture
}

var pagedGLCache sync.Map // *pixel.PagedPictureData -> *glPagedPicture

func newGLPagedPicture(pp *pixel.PagedPictureData) *glPagedPicture {
	if cached, found := pagedGLCache.Load(pp); found {
		return cached.(*glPagedPicture)
	}
	gp := &glPagedPicture{src: pp}
	for _, page := range pp.Pages() {
		g := NewGLPicture(page)
		gp.pages = append(gp.pages, g)
		gp.texs = append(gp.texs, g.Texture())
	}
	pagedGLCache.Store(pp, gp)
	return gp
}

func (gp *glPagedPicture) Bounds() pixel.Rect {
	return gp.src.Bounds()
}

func (gp *glPagedPicture) Texture() *glhf.Texture {
	return gp.texs[0]
}

func (gp *glPagedPicture) Color(at pixel.Vec) pixel.RGBA {
	return gp.src.Color(at)
}
