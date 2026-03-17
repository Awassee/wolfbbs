package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	Esc = "\x1b"

	Reset = "\x1b[0m"
	Bold  = "\x1b[1m"

	FgBlack   = "\x1b[30m"
	FgRed     = "\x1b[31m"
	FgGreen   = "\x1b[32m"
	FgYellow  = "\x1b[33m"
	FgBlue    = "\x1b[34m"
	FgMagenta = "\x1b[35m"
	FgCyan    = "\x1b[36m"
	FgWhite   = "\x1b[37m"
	FgOrange  = "\x1b[38;5;208m"

	BgBlack = "\x1b[40m"
	BgBlue  = "\x1b[44m"
	BgCyan  = "\x1b[46m"
	BgGray  = "\x1b[100m"
)

type BorderSet struct {
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	Horizontal  string
	Vertical    string
	LeftT       string
	RightT      string
}

var CP437Box = BorderSet{
	TopLeft:     "╔",
	TopRight:    "╗",
	BottomLeft:  "╚",
	BottomRight: "╝",
	Horizontal:  "═",
	Vertical:    "║",
	LeftT:       "╠",
	RightT:      "╣",
}

var AsciiBox = BorderSet{
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
	Horizontal:  "-",
	Vertical:    "|",
	LeftT:       "+",
	RightT:      "+",
}

func ClearScreen() string {
	return Esc + "[2J" + Esc + "[H"
}

func MoveCursor(row, col int) string {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("%s[%d;%dH", Esc, row, col)
}

func Color(fg, bg string, body string) string {
	return fg + bg + body + Reset
}

func CenterText(width int, text string) string {
	if width <= 0 {
		return text
	}
	text = strings.TrimRight(text, "\r\n")
	text = trimANSIVisible(text, width)
	if visibleRuneLen(text) >= width {
		return text
	}
	pad := width - visibleRuneLen(text)
	left := pad / 2
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", pad-left)
}

func CenterTextLine(width int, fg, bg, body string) string {
	return fg + bg + CenterText(width, body) + Reset
}

func DrawBox(width, height int, title string, content []string, b BorderSet, fg, bg string) string {
	if width < 4 {
		width = 4
	}
	if height < 2 {
		height = 2
	}
	innerWidth := width - 2
	var lines []string
	header := " " + strings.TrimSpace(title) + " "
	header = trimRunes(header, innerWidth)
	headerPad := innerWidth - runeLen(header)
	headerLeft := headerPad / 2
	headerLine := b.TopLeft + strings.Repeat(b.Horizontal, headerLeft) + header + strings.Repeat(b.Horizontal, innerWidth-runeLen(header)-headerLeft) + b.TopRight
	lines = append(lines, Color(fg, bg, headerLine))
	for i := 0; i < height-2; i++ {
		contentLine := ""
		if i < len(content) {
			contentLine = content[i]
		}
		contentLine = trimANSIVisible(contentLine, innerWidth)
		padding := innerWidth - visibleRuneLen(contentLine)
		if padding < 0 {
			padding = 0
		}
		line := fg + bg + b.Vertical + Reset + bg + contentLine + strings.Repeat(" ", padding) + fg + bg + b.Vertical + Reset
		lines = append(lines, line)
	}
	lines = append(lines, Color(fg, bg, b.BottomLeft+strings.Repeat(b.Horizontal, innerWidth)+b.BottomRight))
	return strings.Join(lines, "\r\n") + "\r\n"
}

func FooterPrompt(width int, text string) string {
	if width <= 0 {
		width = 80
	}
	strip := strings.TrimRight(text, "\r\n")
	strip = trimANSIVisible(strip, width)
	label := "-- " + strip + " --"
	label = trimANSIVisible(label, width)
	pad := width - visibleRuneLen(label)
	left := pad / 2
	return strings.Repeat(" ", left) + label + strings.Repeat(" ", pad-left)
}

func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}

func trimRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(value) <= width {
		return value
	}
	r := []rune(value)
	if width >= len(r) {
		return value
	}
	return string(r[:width])
}

func visibleRuneLen(value string) int {
	return runeLen(stripANSIEscapes(value))
}

func trimANSIVisible(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleRuneLen(value) <= width {
		return value
	}
	var out strings.Builder
	visible := 0
	for i := 0; i < len(value); {
		if value[i] == 0x1b {
			j := i + 1
			if j < len(value) && value[j] == '[' {
				j++
				for j < len(value) {
					ch := value[j]
					if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
						j++
						break
					}
					j++
				}
				out.WriteString(value[i:j])
				i = j
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 1 {
			if visible >= width {
				break
			}
			out.WriteByte(value[i])
			visible++
			i++
			continue
		}
		if visible >= width {
			break
		}
		out.WriteString(value[i : i+size])
		visible++
		i += size
	}
	return out.String()
}
