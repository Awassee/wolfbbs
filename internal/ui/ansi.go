package ui

<<<<<<< ours
import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	Esc = "\x1b"

=======
import "fmt"

const (
>>>>>>> theirs
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
<<<<<<< ours
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
	text = trimRunes(text, width)
	if runeLen(text) >= width {
		return text
	}
	pad := width - runeLen(text)
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
		contentLine = trimRunes(contentLine, innerWidth)
		lines = append(lines, Color(fg, bg, b.Vertical+contentLine+strings.Repeat(" ", innerWidth-runeLen(contentLine))+b.Vertical))
	}
	lines = append(lines, Color(fg, bg, b.BottomLeft+strings.Repeat(b.Horizontal, innerWidth)+b.BottomRight))
	return strings.Join(lines, "\r\n") + "\r\n"
}

func FooterPrompt(width int, text string) string {
	if width <= 0 {
		width = 80
	}
	strip := strings.TrimRight(text, "\r\n")
	strip = trimRunes(strip, width)
	label := "-- " + strip + " --"
	label = trimRunes(label, width)
	pad := width - runeLen(label)
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
=======

	BgBlue  = "\x1b[44m"
	BgBlack = "\x1b[40m"
)

func ClearScreen() string {
	return "\x1b[2J\x1b[H"
}

func MoveCursor(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
>>>>>>> theirs
}
