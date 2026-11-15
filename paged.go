package pixel

import "fmt"

// PagedPictureColumns is how many pages a PagedPictureData lays out side by
// side before starting a new row.
const PagedPictureColumns = 4

// PagedPictureMaxPages is the most pages a PagedPictureData may hold: the
// number of textures an OpenGL canvas binds for one draw.
const PagedPictureMaxPages = 8

// PagedPictureData is one Picture made of several same-size pages, so a
// texture atlas can outgrow the largest texture a device supports and still
// be drawn through one Batch, in any order, in one draw call.
//
// The pages sit in a grid in the Picture's coordinate space: page i covers
// the rectangle whose bottom-left corner is PageOrigin(i), PagedPictureColumns
// pages to a row. A Sprite or a text atlas frame refers to a rectangle inside
// one page. The OpenGL backend uploads each page as its own texture and its
// fragment shader picks the page from the texture coordinate.
type PagedPictureData struct {
	pages    []*PictureData
	pageSize Vec
}

// NewPagedPictureData builds a PagedPictureData from pages, which must all
// have the same size with their bounds starting at (0, 0).
func NewPagedPictureData(pages []*PictureData) *PagedPictureData {
	if len(pages) == 0 || len(pages) > PagedPictureMaxPages {
		panic(fmt.Errorf("NewPagedPictureData: %d pages, want 1 to %d", len(pages), PagedPictureMaxPages))
	}
	size := pages[0].Bounds().Size()
	for i, p := range pages {
		if p.Bounds().Min != ZV || p.Bounds().Size() != size {
			panic(fmt.Errorf("NewPagedPictureData: page %d is %v, want %v at the origin", i, p.Bounds(), size))
		}
	}
	return &PagedPictureData{pages: pages, pageSize: size}
}

// Pages returns the pages, in order.
func (pp *PagedPictureData) Pages() []*PictureData {
	return pp.pages
}

// PageSize returns the width and height of every page.
func (pp *PagedPictureData) PageSize() Vec {
	return pp.pageSize
}

// PageOrigin returns where page i's bottom-left corner sits in the Picture.
func (pp *PagedPictureData) PageOrigin(i int) Vec {
	return V(float64(i%PagedPictureColumns)*pp.pageSize.X, float64(i/PagedPictureColumns)*pp.pageSize.Y)
}

// Bounds returns the rectangle that holds every page.
func (pp *PagedPictureData) Bounds() Rect {
	cols := min(len(pp.pages), PagedPictureColumns)
	rows := (len(pp.pages) + PagedPictureColumns - 1) / PagedPictureColumns
	return R(0, 0, float64(cols)*pp.pageSize.X, float64(rows)*pp.pageSize.Y)
}

// Color returns the color at a position in the Picture, or transparent
// outside every page.
func (pp *PagedPictureData) Color(at Vec) RGBA {
	if at.X < 0 || at.Y < 0 {
		return RGBA{}
	}
	col := int(at.X / pp.pageSize.X)
	row := int(at.Y / pp.pageSize.Y)
	if col >= PagedPictureColumns {
		return RGBA{}
	}
	i := row*PagedPictureColumns + col
	if i >= len(pp.pages) {
		return RGBA{}
	}
	return pp.pages[i].Color(at.Sub(pp.PageOrigin(i)))
}
