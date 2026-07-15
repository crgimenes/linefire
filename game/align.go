package game

import "image/color"

// alignment classifies an entity for the map markers: good (a beneficial item),
// bad (hostile / harmful), ally (a friendly unit), or neutral.
type alignment int

const (
	alignNeutral alignment = iota
	alignGood
	alignBad
	alignAlly
)

var (
	markerGood    = color.RGBA{0x60, 0xff, 0x90, 0xff} // green — good item
	markerBad     = color.RGBA{0xff, 0x55, 0x55, 0xff} // red — bad / enemy
	markerAlly    = color.RGBA{0x55, 0xa0, 0xff, 0xff} // blue — ally
	markerNeutral = color.RGBA{0xff, 0xe0, 0x40, 0xff} // yellow — neutral
)

// markerColor returns the map-marker color for an alignment.
func markerColor(a alignment) color.RGBA {
	switch a {
	case alignGood:
		return markerGood
	case alignBad:
		return markerBad
	case alignAlly:
		return markerAlly
	default:
		return markerNeutral
	}
}
