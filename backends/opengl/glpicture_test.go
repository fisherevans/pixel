package opengl

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/gopxl/pixel/v2"
)

func TestPictureDataUploadBytes(t *testing.T) {
	data := &pixel.PictureData{
		Rect:   pixel.R(0, 0, 1, 2),
		Stride: 2,
		Pix:    []color.RGBA{{R: 1, G: 2, B: 3, A: 4}, {R: 20}, {R: 5, G: 6, B: 7, A: 8}, {R: 30}},
	}
	tests := []struct {
		name     string
		width    int
		want     []byte
		borrowed bool
	}{
		{"contiguous", 2, []byte{1, 2, 3, 4, 20, 0, 0, 0, 5, 6, 7, 8, 30, 0, 0, 0}, true},
		{"padded", 1, []byte{1, 2, 3, 4, 5, 6, 7, 8}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pictureDataUploadBytes(data, tt.width, 2)
			if !bytes.Equal(result, tt.want) {
				t.Fatalf("upload bytes = %v, want %v", result, tt.want)
			}
			data.Pix[0].R = 99
			defer func() { data.Pix[0].R = 1 }()
			if (result[0] == 99) != tt.borrowed {
				t.Fatalf("unexpected storage alias: upload red is %d", result[0])
			}
		})
	}
}

func TestGLPictureColorReadsSource(t *testing.T) {
	data := pixel.MakePictureData(pixel.R(4, 6, 5, 7))
	data.Pix[0] = color.RGBA{R: 11, G: 23, B: 37, A: 89}
	p := &glPicture{bounds: data.Bounds(), source: data}
	if got, want := p.Color(pixel.V(4, 6)), pixel.ToRGBA(data.Pix[0]); got != want {
		t.Fatalf("pixel = %v, want %v", got, want)
	}
	if got := p.Color(pixel.V(3, 6)); got != (pixel.RGBA{}) {
		t.Fatalf("out of bounds pixel = %v", got)
	}
}
