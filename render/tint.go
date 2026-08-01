package render

import "image/color"

// Retinted returns a copy of the mesh with every path's fill and stroke moved
// to the target's hue, keeping each color's own saturation, value and alpha —
// so a faction can own a hull without flattening the art's shading. A straight
// blend toward the faction color drags saturation with it and washes the art
// out; moving only the hue keeps every highlight and shadow where the artist
// put it. An achromatic target (white or grey) has no hue to move to, so the
// copy desaturates instead, which reads as that faction's scheme.
//
// The vector geometry is shared with the original, not copied: paths are
// immutable at runtime, and only the color table is new.
func (m *Mesh) Retinted(target color.RGBA) *Mesh {
	if m.Empty() {
		return m
	}
	th, ts, _ := rgbToHSV(target)
	out := &Mesh{paths: make([]meshPath, len(m.paths))}
	for i, p := range m.paths {
		p.fill = hueTo(p.fill, th, ts)
		p.stroke = hueTo(p.stroke, th, ts)
		out.paths[i] = p
	}
	return out
}

// hueTo moves c to hue h keeping its saturation and value, or desaturates it
// when the target itself is achromatic (ts ~ 0, where a hue is meaningless).
func hueTo(c color.RGBA, h, ts float64) color.RGBA {
	_, s, v := rgbToHSV(c)
	if ts < 0.01 {
		s = 0
	}
	r, g, b := hsvToRGB(h, s, v)
	return color.RGBA{R: r, G: g, B: b, A: c.A}
}

// rgbToHSV converts to hue [0,360), saturation [0,1] and value [0,1].
func rgbToHSV(c color.RGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	maxc := max(r, g, b)
	minc := min(r, g, b)
	v = maxc
	d := maxc - minc
	if maxc > 0 {
		s = d / maxc
	}
	if d == 0 {
		return 0, s, v
	}
	switch maxc {
	case r:
		h = 60 * (g - b) / d
	case g:
		h = 60*(b-r)/d + 120
	default:
		h = 60*(r-g)/d + 240
	}
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// hsvToRGB is the inverse of rgbToHSV.
func hsvToRGB(h, s, v float64) (r, g, b uint8) {
	c := v * s
	hh := h / 60
	x := c * (1 - abs(mod2(hh)-1))
	var rf, gf, bf float64
	switch {
	case hh < 1:
		rf, gf, bf = c, x, 0
	case hh < 2:
		rf, gf, bf = x, c, 0
	case hh < 3:
		rf, gf, bf = 0, c, x
	case hh < 4:
		rf, gf, bf = 0, x, c
	case hh < 5:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	m := v - c
	return uint8((rf + m) * 255), uint8((gf + m) * 255), uint8((bf + m) * 255)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// mod2 wraps v into [0,2), the hue sector math's period.
func mod2(v float64) float64 {
	for v >= 2 {
		v -= 2
	}
	for v < 0 {
		v += 2
	}
	return v
}
