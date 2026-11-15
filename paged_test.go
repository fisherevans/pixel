package pixel

import "testing"

func TestPagedPictureData(t *testing.T) {
	var pages []*PictureData
	for i := 0; i < 6; i++ {
		p := MakePictureData(R(0, 0, 4, 2))
		for j := range p.Pix {
			p.Pix[j].R = uint8(i + 1)
			p.Pix[j].A = 255
		}
		pages = append(pages, p)
	}
	pp := NewPagedPictureData(pages)
	if got, want := pp.Bounds(), R(0, 0, 16, 4); got != want {
		t.Fatalf("Bounds = %v, want %v", got, want)
	}
	tests := []struct {
		name string
		at   Vec
		want uint8
	}{
		{"page 0", V(0.5, 0.5), 1},
		{"page 3", V(12.5, 1.5), 4},
		{"page 4", V(0.5, 2.5), 5},
		{"page 5", V(7.5, 3.5), 6},
		{"no page 6", V(8.5, 2.5), 0},
		{"outside", V(-1, 0), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uint8(pp.Color(tt.at).R*255 + 0.5); got != tt.want {
				t.Errorf("Color(%v).R = %d, want %d", tt.at, got, tt.want)
			}
		})
	}
	if got, want := pp.PageOrigin(5), V(4, 2); got != want {
		t.Errorf("PageOrigin(5) = %v, want %v", got, want)
	}
}
