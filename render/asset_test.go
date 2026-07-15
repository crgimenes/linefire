package render

import (
	"image/color"
	"testing"
)

func TestParseColor(t *testing.T) {
	cases := []struct {
		in      string
		visible bool
		want    color.RGBA
	}{
		{"#80ffff", true, color.RGBA{0x80, 0xff, 0xff, 0xff}},
		{"#FFFFFF", true, color.RGBA{0xff, 0xff, 0xff, 0xff}},
		{"#0f8", true, color.RGBA{0x00, 0xff, 0x88, 0xff}},
		{"#001820aa", true, color.RGBA{0x00, 0x18, 0x20, 0xaa}},
		{"transparent", false, color.RGBA{}},
		{"", false, color.RGBA{}},
		{"#zz", false, color.RGBA{}},
		{"red", false, color.RGBA{}},
	}
	for _, c := range cases {
		got, visible := ParseColor(c.in)
		if visible != c.visible {
			t.Errorf("ParseColor(%q) visible = %v, want %v", c.in, visible, c.visible)
			continue
		}
		if visible && got != c.want {
			t.Errorf("ParseColor(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestViewProjectUnproject(t *testing.T) {
	v := View{OffsetX: 100, OffsetY: 50, Scale: 8}
	sx, sy := v.Project(10, 20)
	if sx != 180 || sy != 210 {
		t.Fatalf("Project = (%v,%v), want (180,210)", sx, sy)
	}
	ax, ay := v.Unproject(180, 210)
	if ax != 10 || ay != 20 {
		t.Fatalf("Unproject = (%v,%v), want (10,20)", ax, ay)
	}
}
