package game

import "testing"

func TestMarkerColorLegend(t *testing.T) {
	cases := []struct {
		a    alignment
		want [3]uint8 // dominant channel sanity: green / red / blue
	}{
		{alignGood, [3]uint8{0x60, 0xff, 0x90}},
		{alignBad, [3]uint8{0xff, 0x55, 0x55}},
		{alignAlly, [3]uint8{0x55, 0xa0, 0xff}},
		{alignNeutral, [3]uint8{0xff, 0xe0, 0x40}},
	}
	for _, c := range cases {
		got := markerColor(c.a)
		if got.R != c.want[0] || got.G != c.want[1] || got.B != c.want[2] {
			t.Fatalf("markerColor(%d) = %v, want RGB %v", c.a, got, c.want)
		}
	}
}
