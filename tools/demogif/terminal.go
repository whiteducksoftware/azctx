package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/charmbracelet/x/ansi"
)

// cellStyle uses a zero color for the terminal default.
type cellStyle struct {
	fg, bg  color.RGBA
	bold    bool
	reverse bool
}

type cell struct {
	// text is empty for the trailing half of a wide grapheme.
	text  string
	style cellStyle
}

var (
	terminalBackground = color.RGBA{0x0d, 0x11, 0x17, 0xff}
	terminalForeground = color.RGBA{0xc9, 0xd1, 0xd9, 0xff}
	// A GitHub-dark-like terminal palette for the picker's basic ANSI colors.
	ansiColors = [16]color.RGBA{
		{0x48, 0x4f, 0x58, 0xff},
		{0xff, 0x7b, 0x72, 0xff},
		{0x3f, 0xb9, 0x50, 0xff},
		{0xd2, 0x99, 0x22, 0xff},
		{0x58, 0xa6, 0xff, 0xff},
		{0xbc, 0x8c, 0xff, 0xff},
		{0x39, 0xc5, 0xcf, 0xff},
		{0xb1, 0xba, 0xc4, 0xff},
		{0x6e, 0x76, 0x81, 0xff},
		{0xff, 0xa1, 0x98, 0xff},
		{0x56, 0xd3, 0x64, 0xff},
		{0xe3, 0xb3, 0x41, 0xff},
		{0x79, 0xc0, 0xff, 0xff},
		{0xd2, 0xa8, 0xff, 0xff},
		{0x56, 0xd4, 0xdd, 0xff},
		{0xf0, 0xf6, 0xfc, 0xff},
	}
)

func blankScreen(columns, rows int) [][]cell {
	screen := make([][]cell, rows)
	for row := range screen {
		screen[row] = make([]cell, columns)
		for column := range screen[row] {
			screen[row][column].text = " "
		}
	}
	return screen
}

// parseScreen accepts only printable text, newlines and SGR styles. Anything
// else would make the image disagree with a terminal, so it is an error.
func parseScreen(content string, columns, rows int) ([][]cell, error) {
	screen := blankScreen(columns, rows)
	parser := ansi.NewParser()
	var state byte
	var style cellStyle
	row, column := 0, 0
	for len(content) > 0 {
		sequence, width, size, next := ansi.DecodeSequence(content, state, parser)
		state, content = next, content[size:]
		command := ansi.Cmd(parser.Command())
		switch {
		case width > 0:
			if row >= rows || column+width > columns {
				return nil, fmt.Errorf("%q exceeds the %dx%d terminal", sequence, columns, rows)
			}
			screen[row][column] = cell{text: sequence, style: style}
			for offset := 1; offset < width; offset++ {
				screen[row][column+offset] = cell{style: style}
			}
			column += width
		case sequence == "\n":
			row, column = row+1, 0
			if row >= rows {
				return nil, fmt.Errorf("content exceeds %d terminal rows", rows)
			}
		case ansi.HasCsiPrefix(sequence) && command.Final() == 'm' && command.Prefix() == 0 && command.Intermediate() == 0:
			var err error
			if style, err = applySGR(style, parser.Params()); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported terminal sequence %q", sequence)
		}
	}
	return screen, nil
}

func applySGR(style cellStyle, params ansi.Params) (cellStyle, error) {
	values := make([]int, len(params))
	for index, param := range params {
		if param.HasMore() {
			return style, fmt.Errorf("unsupported SGR sub-parameters")
		}
		values[index] = param.Param(0)
	}
	if len(values) == 0 {
		return cellStyle{}, nil
	}
	for index := 0; index < len(values); index++ {
		var err error
		switch value := values[index]; value {
		case 38:
			style.fg, index, err = extendedColor(values, index)
		case 48:
			style.bg, index, err = extendedColor(values, index)
		default:
			style, err = applyBasicSGR(style, value)
		}
		if err != nil {
			return style, err
		}
	}
	return style, nil
}

func applyBasicSGR(style cellStyle, value int) (cellStyle, error) {
	switch {
	case value == 0:
		style = cellStyle{}
	case value == 1 || value == 22:
		style.bold = value == 1
	case value == 7 || value == 27:
		style.reverse = value == 7
	case value >= 30 && value <= 37, value >= 90 && value <= 97, value == 39:
		style.fg = basicColor(value - 30)
	case value >= 40 && value <= 47, value >= 100 && value <= 107, value == 49:
		style.bg = basicColor(value - 40)
	default:
		return style, fmt.Errorf("unsupported SGR parameter %d", value)
	}
	return style, nil
}

// basicColor maps an SGR offset (0-7, 9 for default, 60-67 for bright).
func basicColor(offset int) color.RGBA {
	switch {
	case offset == 9:
		return color.RGBA{}
	case offset >= 60:
		return ansiColors[offset-60+8]
	default:
		return ansiColors[offset]
	}
}

func extendedColor(values []int, index int) (color.RGBA, int, error) {
	switch {
	case index+2 < len(values) && values[index+1] == 5 && values[index+2] < 256:
		return xterm256(values[index+2]), index + 2, nil
	case index+4 < len(values) && values[index+1] == 2 && max(values[index+2], values[index+3], values[index+4]) < 256:
		rgb := values[index+2 : index+5]
		return color.RGBA{toByte(rgb[0]), toByte(rgb[1]), toByte(rgb[2]), 0xff}, index + 4, nil
	default:
		return color.RGBA{}, index, fmt.Errorf("unsupported extended color %v", values[index:])
	}
}

func xterm256(value int) color.RGBA {
	if value < 16 {
		return ansiColors[value]
	}
	if value >= 232 {
		gray := toByte(8 + 10*(value-232))
		return color.RGBA{gray, gray, gray, 0xff}
	}
	levels := [6]uint8{0, 95, 135, 175, 215, 255}
	value -= 16
	return color.RGBA{levels[value/36], levels[value/6%6], levels[value%6], 0xff}
}

// toByte converts values whose range the caller has already checked.
func toByte(value int) uint8 {
	if value < 0 || value > math.MaxUint8 {
		panic(fmt.Sprintf("color component %d is outside 0..255", value))
	}
	return uint8(value)
}
