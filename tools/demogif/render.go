package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const (
	cellWidth     = 9
	cellHeight    = 19
	fontSize      = 15
	sidePadding   = 18
	titleHeight   = 34
	termPadding   = 12
	captionHeight = 44
	// Antialiasing shades per foreground/background pair, including both ends.
	shades = 8
	title  = "azctx built-in picker · synthetic demo data · not a live Azure account"
)

var (
	chromeBackground = color.RGBA{0x16, 0x1b, 0x22, 0xff}
	chromeBorder     = color.RGBA{0x30, 0x36, 0x3d, 0xff}
	mutedText        = color.RGBA{0x8b, 0x94, 0x9e, 0xff}
	windowDots       = [3]color.RGBA{{0xff, 0x5f, 0x57, 0xff}, {0xfe, 0xbc, 0x2e, 0xff}, {0x28, 0xc8, 0x40, 0xff}}
)

func canvasSize() (width, height int) {
	return 2*sidePadding + columns*cellWidth, titleHeight + 2*termPadding + (pickerRows+1)*cellHeight + captionHeight
}

type glyphKey struct {
	character rune
	bold      bool
}

type renderer struct {
	regular, bold *sfnt.Font
	baseline      int
	buffer        sfnt.Buffer
	glyphs        map[glyphKey]glyphMask
	palette       color.Palette
	indexes       map[color.RGBA]uint8
}

func newRenderer() (*renderer, error) {
	regular, err := sfnt.Parse(gomono.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := sfnt.Parse(gomonobold.TTF)
	if err != nil {
		return nil, err
	}
	r := &renderer{regular: regular, bold: bold, glyphs: map[glyphKey]glyphMask{}, indexes: map[color.RGBA]uint8{}}
	metrics, err := regular.Metrics(&r.buffer, fixed.I(fontSize), font.HintingFull)
	if err != nil {
		return nil, err
	}
	r.baseline = (cellHeight-(metrics.Ascent+metrics.Descent).Ceil())/2 + metrics.Ascent.Ceil()
	return r, nil
}

func (r *renderer) glyph(key glyphKey) (glyphMask, error) {
	if mask, ok := r.glyphs[key]; ok {
		return mask, nil
	}
	face := r.regular
	if key.bold {
		face = r.bold
	}
	index, err := face.GlyphIndex(&r.buffer, key.character)
	if err != nil || index == 0 {
		return glyphMask{}, fmt.Errorf("font has no glyph for %q", key.character)
	}
	segments, err := face.LoadGlyph(&r.buffer, index, fixed.I(fontSize), nil)
	if err != nil {
		return glyphMask{}, err
	}
	edges, err := glyphEdges(segments)
	if err != nil {
		return glyphMask{}, err
	}
	mask, err := rasterize(edges)
	if err != nil {
		return glyphMask{}, fmt.Errorf("%q: %w", key.character, err)
	}
	r.glyphs[key] = mask
	return mask, nil
}

// colorIndex grows one palette in first-use order, so output stays deterministic.
func (r *renderer) colorIndex(value color.RGBA) (uint8, error) {
	if index, ok := r.indexes[value]; ok {
		return index, nil
	}
	if len(r.palette) == 256 {
		return 0, errors.New("demo needs more than 256 colors")
	}
	index := toByte(len(r.palette))
	r.palette = append(r.palette, value)
	r.indexes[value] = index
	return index, nil
}

func (r *renderer) fill(target *image.Paletted, rect image.Rectangle, value color.RGBA) error {
	index, err := r.colorIndex(value)
	if err != nil {
		return err
	}
	rect = rect.Intersect(target.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			target.SetColorIndex(x, y, index)
		}
	}
	return nil
}

func resolve(style cellStyle) (fg, bg color.RGBA) {
	fg, bg = style.fg, style.bg
	if fg.A == 0 {
		fg = terminalForeground
	}
	if bg.A == 0 {
		bg = terminalBackground
	}
	if style.reverse {
		fg, bg = bg, fg
	}
	return fg, bg
}

func cellRect(origin image.Point, row, column int) image.Rectangle {
	minimum := origin.Add(image.Pt(column*cellWidth, row*cellHeight))
	return image.Rectangle{Min: minimum, Max: minimum.Add(image.Pt(cellWidth, cellHeight))}
}

// drawCells paints every background first so glyph overhangs are not erased.
func (r *renderer) drawCells(target *image.Paletted, screen [][]cell, origin image.Point) error {
	for row, line := range screen {
		for column, current := range line {
			_, bg := resolve(current.style)
			if err := r.fill(target, cellRect(origin, row, column), bg); err != nil {
				return err
			}
		}
	}
	for row, line := range screen {
		for column, current := range line {
			if err := r.drawGlyph(target, cellRect(origin, row, column), current); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *renderer) drawGlyph(target *image.Paletted, rect image.Rectangle, current cell) error {
	if current.text == "" || current.text == " " {
		return nil
	}
	character, size := utf8.DecodeRuneInString(current.text)
	if size != len(current.text) {
		return fmt.Errorf("unsupported grapheme cluster %q", current.text)
	}
	fg, bg := resolve(current.style)
	if character == '▸' {
		// Go Mono has no small triangle; draw the picker's marker directly.
		return r.drawMarker(target, rect, fg)
	}
	mask, err := r.glyph(glyphKey{character, current.style.bold})
	if err != nil {
		return err
	}
	origin := image.Pt(rect.Min.X, rect.Min.Y+r.baseline)
	bounds := mask.bounds.Add(origin).Intersect(target.Bounds())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			shade := (mask.at(x-origin.X, y-origin.Y)*(shades-1) + maxCoverage/2) / maxCoverage
			if shade == 0 {
				continue
			}
			index, err := r.colorIndex(blend(bg, fg, shade))
			if err != nil {
				return err
			}
			target.SetColorIndex(x, y, index)
		}
	}
	return nil
}

func blend(bg, fg color.RGBA, shade int) color.RGBA {
	mix := func(from, to uint8) uint8 {
		return toByte((int(from)*(shades-1-shade) + int(to)*shade + (shades-1)/2) / (shades - 1))
	}
	return color.RGBA{mix(bg.R, fg.R), mix(bg.G, fg.G), mix(bg.B, fg.B), 0xff}
}

func (r *renderer) drawMarker(target *image.Paletted, rect image.Rectangle, fg color.RGBA) error {
	index, err := r.colorIndex(fg)
	if err != nil {
		return err
	}
	const half = 4
	centerY := rect.Min.Y + r.baseline - half
	for dy := -half; dy <= half; dy++ {
		span := (half - max(dy, -dy)) * 6 / half
		for dx := 0; dx <= span; dx++ {
			target.SetColorIndex(rect.Min.X+1+dx, centerY+dy, index)
		}
	}
	return nil
}

func textCells(value string, style cellStyle) []cell {
	cells := make([]cell, 0, len(value))
	for _, character := range value {
		cells = append(cells, cell{text: string(character), style: style})
	}
	return cells
}

func (r *renderer) drawRow(target *image.Paletted, row []cell, origin image.Point) error {
	if len(row) > columns {
		return fmt.Errorf("%d cells exceed %d columns", len(row), columns)
	}
	return r.drawCells(target, [][]cell{row}, origin)
}

func (r *renderer) drawChrome(target *image.Paletted) error {
	width, height := canvasSize()
	if err := r.fill(target, target.Bounds(), terminalBackground); err != nil {
		return err
	}
	for _, band := range []struct {
		rect   image.Rectangle
		border int
	}{
		{image.Rect(0, 0, width, titleHeight), titleHeight - 1},
		{image.Rect(0, height-captionHeight, width, height), height - captionHeight},
	} {
		if err := r.fill(target, band.rect, chromeBackground); err != nil {
			return err
		}
		if err := r.fill(target, image.Rect(0, band.border, width, band.border+1), chromeBorder); err != nil {
			return err
		}
	}
	for index, dot := range windowDots {
		if err := r.drawDot(target, image.Pt(sidePadding+6+index*20, titleHeight/2), dot); err != nil {
			return err
		}
	}
	titleStyle := cellStyle{fg: mutedText, bg: chromeBackground}
	column := (columns - utf8.RuneCountInString(title)) / 2
	return r.drawRow(target, textCells(title, titleStyle), image.Pt(sidePadding+column*cellWidth, (titleHeight-cellHeight)/2))
}

func (r *renderer) drawDot(target *image.Paletted, center image.Point, value color.RGBA) error {
	index, err := r.colorIndex(value)
	if err != nil {
		return err
	}
	const radius = 6
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius+radius/2 {
				target.SetColorIndex(center.X+dx, center.Y+dy, index)
			}
		}
	}
	return nil
}

func captionCells(value caption) []cell {
	keyStyle := cellStyle{fg: terminalBackground, bg: ansiColors[4], bold: true}
	textStyle := cellStyle{fg: terminalForeground, bg: chromeBackground}
	var row []cell
	for _, key := range value.keys {
		row = append(row, textCells(" "+key+" ", keyStyle)...)
		row = append(row, textCells(" ", textStyle)...)
	}
	return append(row, textCells(value.text, textStyle)...)
}

func (r *renderer) drawFrame(target *image.Paletted, current frame) error {
	if err := r.drawChrome(target); err != nil {
		return err
	}
	terminal := image.Pt(sidePadding, titleHeight+termPadding)
	prompt := append(textCells("$ ", cellStyle{fg: mutedText}), textCells(command, cellStyle{bold: true})...)
	if err := r.drawRow(target, prompt, terminal); err != nil {
		return err
	}
	screen, err := parseScreen(current.content, columns, pickerRows)
	if err != nil {
		return err
	}
	if err := r.drawCells(target, screen, terminal.Add(image.Pt(0, cellHeight))); err != nil {
		return err
	}
	_, height := canvasSize()
	return r.drawRow(target, captionCells(current.caption), image.Pt(sidePadding, height-captionHeight+(captionHeight-cellHeight)/2))
}

func changedBounds(previous, current *image.Paletted) image.Rectangle {
	var changed image.Rectangle
	bounds := current.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if previous.ColorIndexAt(x, y) != current.ColorIndexAt(x, y) {
				changed = changed.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return changed
}

// generate renders full frames, then stores only each frame's changed rectangle.
func generate() ([]byte, error) {
	frames, _, err := captureFrames()
	if err != nil {
		return nil, err
	}
	r, err := newRenderer()
	if err != nil {
		return nil, err
	}
	width, height := canvasSize()
	images := make([]*image.Paletted, len(frames))
	for index, current := range frames {
		images[index] = image.NewPaletted(image.Rect(0, 0, width, height), nil)
		if err := r.drawFrame(images[index], current); err != nil {
			return nil, fmt.Errorf("frame %d: %w", index, err)
		}
	}
	animation := &gif.GIF{Config: image.Config{ColorModel: r.palette, Width: width, Height: height}}
	for index, current := range images {
		current.Palette = r.palette
		bounds := current.Bounds()
		if index > 0 {
			if bounds = changedBounds(images[index-1], current); bounds.Empty() {
				return nil, fmt.Errorf("frame %d does not change the animation", index)
			}
		}
		sub, ok := current.SubImage(bounds).(*image.Paletted)
		if !ok {
			return nil, errors.New("unexpected frame image type")
		}
		animation.Image = append(animation.Image, sub)
		animation.Delay = append(animation.Delay, frames[index].delay)
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, animation); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
