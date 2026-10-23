package main

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"slices"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Glyphs are rasterized with integer-only supersampling. Floating-point
// rasterizers can round differently across architectures, which would make the
// committed GIF bytes depend on the machine that generated them.
const (
	subsamples   = 8
	curveSteps   = 8
	maxCoverage  = subsamples * subsamples
	subpixelStep = 64 / subsamples
)

type point struct{ x, y int }

type edge struct{ from, to point }

// glyphMask holds 0..maxCoverage samples per pixel, offset from the baseline origin.
type glyphMask struct {
	bounds   image.Rectangle
	coverage []uint8
}

func (mask glyphMask) at(x, y int) int {
	return int(mask.coverage[(y-mask.bounds.Min.Y)*mask.bounds.Dx()+x-mask.bounds.Min.X])
}

func glyphEdges(segments sfnt.Segments) ([]edge, error) {
	var edges []edge
	var start, pen point
	closeContour := func() {
		if pen != start {
			edges = append(edges, edge{pen, start})
		}
	}
	for _, segment := range segments {
		args := segment.Args
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			closeContour()
			start, pen = fixedPoint(args[0]), fixedPoint(args[0])
		case sfnt.SegmentOpLineTo:
			next := fixedPoint(args[0])
			edges, pen = append(edges, edge{pen, next}), next
		case sfnt.SegmentOpQuadTo:
			control, end := fixedPoint(args[0]), fixedPoint(args[1])
			previous := pen
			for step := 1; step <= curveSteps; step++ {
				next := quadPoint(pen, control, end, step)
				edges, previous = append(edges, edge{previous, next}), next
			}
			pen = end
		default:
			return nil, fmt.Errorf("unsupported glyph segment %d", segment.Op)
		}
	}
	closeContour()
	return edges, nil
}

func fixedPoint(value fixed.Point26_6) point { return point{int(value.X), int(value.Y)} }

func quadPoint(from, control, to point, step int) point {
	rest := curveSteps - step
	weight := func(a, b, c int) int {
		return roundDiv(rest*rest*a+2*step*rest*b+step*step*c, curveSteps*curveSteps)
	}
	return point{weight(from.x, control.x, to.x), weight(from.y, control.y, to.y)}
}

func roundDiv(value, divisor int) int {
	if value >= 0 {
		return (value + divisor/2) / divisor
	}
	return -((-value + divisor/2) / divisor)
}

func floorDiv(value, divisor int) int {
	quotient := value / divisor
	if value%divisor != 0 && (value < 0) != (divisor < 0) {
		quotient--
	}
	return quotient
}

type crossing struct{ x, winding int }

// rasterize applies the nonzero winding rule at subsamples×subsamples points per pixel.
func rasterize(edges []edge) (glyphMask, error) {
	if len(edges) == 0 {
		return glyphMask{}, errors.New("glyph has no outline")
	}
	low, high := edges[0].from, edges[0].from
	for _, current := range edges {
		for _, p := range []point{current.from, current.to} {
			low = point{min(low.x, p.x), min(low.y, p.y)}
			high = point{max(high.x, p.x), max(high.y, p.y)}
		}
	}
	bounds := image.Rect(floorDiv(low.x, 64), floorDiv(low.y, 64), floorDiv(high.x+63, 64), floorDiv(high.y+63, 64))
	mask := glyphMask{bounds: bounds, coverage: make([]uint8, bounds.Dx()*bounds.Dy())}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for row := range subsamples {
			sampleY := y*64 + row*subpixelStep + subpixelStep/2
			mask.fillSpans(y, scanline(edges, sampleY))
		}
	}
	return mask, nil
}

func scanline(edges []edge, sampleY int) []crossing {
	var crossings []crossing
	for _, current := range edges {
		from, to, winding := current.from, current.to, 1
		if from.y > to.y {
			from, to, winding = to, from, -1
		}
		if from.y == to.y || sampleY < from.y || sampleY >= to.y {
			continue
		}
		x := from.x + floorDiv((sampleY-from.y)*(to.x-from.x), to.y-from.y)
		crossings = append(crossings, crossing{x, winding})
	}
	slices.SortFunc(crossings, func(a, b crossing) int { return cmp.Or(cmp.Compare(a.x, b.x), cmp.Compare(a.winding, b.winding)) })
	return crossings
}

func (mask glyphMask) fillSpans(y int, crossings []crossing) {
	winding := 0
	for index, current := range crossings {
		before := winding
		winding += current.winding
		if before == 0 {
			continue
		}
		// Samples sit at x*64 + step/2 + n*step; count those in [start, end).
		first := floorDiv(crossings[index-1].x-subpixelStep/2+subpixelStep-1, subpixelStep)
		last := floorDiv(current.x-subpixelStep/2+subpixelStep-1, subpixelStep)
		for sample := first; sample < last; sample++ {
			x := floorDiv(sample, subsamples)
			if x >= mask.bounds.Min.X && x < mask.bounds.Max.X {
				mask.coverage[(y-mask.bounds.Min.Y)*mask.bounds.Dx()+x-mask.bounds.Min.X]++
			}
		}
	}
}
