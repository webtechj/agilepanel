// Package ui provides terminal rendering helpers for AgilePanel CLI output.
// It uses ANSI escape codes for color and Unicode box-drawing characters for
// structured, professional output that works on any modern VPS terminal.
package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ─── ANSI color codes ────────────────────────────────────────────────────────

const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Foreground
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bright foreground
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"
)

// ─── Semantic aliases ─────────────────────────────────────────────────────────

func Accent(s string) string   { return BrightCyan + Bold + s + Reset }
func Success(s string) string  { return BrightGreen + s + Reset }
func Warning(s string) string  { return BrightYellow + s + Reset }
func Danger(s string) string   { return BrightRed + s + Reset }
func Muted(s string) string    { return BrightBlack + s + Reset }
func Header(s string) string   { return BrightWhite + Bold + s + Reset }
func Label(s string) string    { return Cyan + s + Reset }
func Value(s string) string    { return White + s + Reset }
func KeyStr(s string) string   { return BrightBlue + Bold + s + Reset }

// ─── Box-drawing constants ───────────────────────────────────────────────────

const (
	boxW  = 72 // total inner width of printed boxes
	hline = "─"
	vline = "│"
	tlc   = "╭" // top-left corner
	trc   = "╮" // top-right corner
	blc   = "╰" // bottom-left corner
	brc   = "╯" // bottom-right corner
	midL  = "├"
	midR  = "┤"
	cross = "┼"
	teeT  = "┬"
	teeB  = "┴"
)

// repeat returns s repeated n times.
func repeat(s string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(s)
	}
	return b.String()
}

// visLen returns the visible rune length of s, stripping ANSI escape sequences.
func visLen(s string) int {
	stripped := stripANSI(s)
	return utf8.RuneCountInString(stripped)
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		out.WriteString(s[i : i+size])
		i += size
	}
	return out.String()
}

// padRight pads s to width w using spaces (respecting ANSI escape codes).
func padRight(s string, w int) string {
	l := visLen(s)
	if l >= w {
		return s
	}
	return s + strings.Repeat(" ", w-l)
}

// ─── Section headers / dividers ──────────────────────────────────────────────

// Banner prints a full-width box with a centered title line.
//
//	╭──────────────────────────────────────────────────────────────────────╮
//	│                      TITLE TEXT HERE                                │
//	╰──────────────────────────────────────────────────────────────────────╯
func Banner(title string) {
	inner := boxW
	top := BrightBlue + tlc + repeat(hline, inner) + trc + Reset
	bot := BrightBlue + blc + repeat(hline, inner) + brc + Reset

	titleClean := stripANSI(title)
	pad := inner - len(titleClean)
	lpad := pad / 2
	rpad := pad - lpad
	mid := BrightBlue + vline + Reset +
		strings.Repeat(" ", lpad) +
		BrightWhite + Bold + title + Reset +
		strings.Repeat(" ", rpad) +
		BrightBlue + vline + Reset

	fmt.Println()
	fmt.Println(top)
	fmt.Println(mid)
	fmt.Println(bot)
}

// SectionHeader prints a subdued divider line with a label.
//
//	├─── DATABASE ──────────────────────────────────────────────────────────┤
func SectionHeader(label string) {
	inner := boxW
	labelFmt := " " + BrightBlue + Bold + label + Reset + " "
	labelLen := 2 + len(stripANSI(label)) + 1 // " LABEL "
	dashes := inner - labelLen - 4             // 4 for "├───" and "┤"
	if dashes < 0 {
		dashes = 0
	}
	line := BrightBlack + midL + repeat(hline, 3) + Reset +
		labelFmt +
		BrightBlack + repeat(hline, dashes) + midR + Reset
	fmt.Println(line)
}

// Divider prints a thin horizontal rule.
func Divider() {
	fmt.Println(BrightBlack + repeat(hline, boxW+2) + Reset)
}

// ─── Key/value row ───────────────────────────────────────────────────────────

const labelWidth = 22

// Row prints a single key: value line with consistent alignment.
func Row(key, val string) {
	keyFmt := padRight(Cyan+key+Reset, labelWidth+len(Cyan)+len(Reset))
	fmt.Printf("  %s  %s\n", keyFmt, Value(val))
}

// RowBadge prints a key: [BADGE] line where the badge is coloured.
func RowBadge(key, badge, color string) {
	keyFmt := padRight(Cyan+key+Reset, labelWidth+len(Cyan)+len(Reset))
	fmt.Printf("  %s  %s%s%s\n", keyFmt, color, badge, Reset)
}

// ─── Table renderer ──────────────────────────────────────────────────────────

// TableColumn defines a column in a table.
type TableColumn struct {
	Header string
	Width  int // minimum width (content may exceed it)
}

// PrintTable renders a table with a header row and data rows.
// headers is a slice of TableColumn; rows is a slice of string slices.
func PrintTable(columns []TableColumn, rows [][]string) {
	// Compute final widths
	widths := make([]int, len(columns))
	for i, c := range columns {
		widths[i] = len(c.Header)
		if c.Width > widths[i] {
			widths[i] = c.Width
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(stripANSI(cell)) > widths[i] {
				widths[i] = len(stripANSI(cell))
			}
		}
	}

	// Top border
	topLine := BrightBlue + tlc
	for i, w := range widths {
		topLine += repeat(hline, w+2)
		if i < len(widths)-1 {
			topLine += teeT
		}
	}
	topLine += trc + Reset
	fmt.Println(topLine)

	// Header row
	headerLine := BrightBlue + vline + Reset
	for i, col := range columns {
		cell := padRight(BrightWhite+Bold+col.Header+Reset, widths[i])
		headerLine += " " + cell + " " + BrightBlue + vline + Reset
	}
	fmt.Println(headerLine)

	// Header/body separator
	sepLine := BrightBlue + midL
	for i, w := range widths {
		sepLine += repeat(hline, w+2)
		if i < len(widths)-1 {
			sepLine += cross
		}
	}
	sepLine += midR + Reset
	fmt.Println(sepLine)

	// Data rows
	for _, row := range rows {
		rowLine := BrightBlue + vline + Reset
		for i, w := range widths {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			paddedCell := padRight(cell, w+len(cell)-len(stripANSI(cell)))
			rowLine += " " + paddedCell + " " + BrightBlue + vline + Reset
		}
		fmt.Println(rowLine)
	}

	// Bottom border
	botLine := BrightBlue + blc
	for i, w := range widths {
		botLine += repeat(hline, w+2)
		if i < len(widths)-1 {
			botLine += teeB
		}
	}
	botLine += brc + Reset
	fmt.Println(botLine)
}

// ─── Status / badge helpers ──────────────────────────────────────────────────

func StatusActive() string  { return BrightGreen + "● Active" + Reset }
func StatusLocked() string  { return BrightYellow + "⊘ Locked" + Reset }
func StatusInactive() string { return BrightRed + "○ Inactive" + Reset }
func StatusOK() string      { return BrightGreen + "✔" + Reset }
func StatusFail() string    { return BrightRed + "✘" + Reset }

// ─── Result boxes ────────────────────────────────────────────────────────────

// Success box printed after a successful operation.
func PrintSuccess(title string) {
	inner := boxW
	top := BrightGreen + tlc + repeat(hline, inner) + trc + Reset
	bot := BrightGreen + blc + repeat(hline, inner) + brc + Reset
	titleClean := "  " + StatusOK() + "  " + BrightWhite + Bold + title + Reset + "  "
	padTotal := inner - visLen(titleClean)
	if padTotal < 0 {
		padTotal = 0
	}
	mid := BrightGreen + vline + Reset + titleClean + strings.Repeat(" ", padTotal) + BrightGreen + vline + Reset
	fmt.Println()
	fmt.Println(top)
	fmt.Println(mid)
	fmt.Println(bot)
}

// PrintWarning prints a single warning line.
func PrintWarning(msg string) {
	fmt.Printf("  %s  %s\n", Warning("⚠ Warning:"), Muted(msg))
}

// PrintStep prints a numbered step line during a long operation.
func PrintStep(n int, msg string) {
	fmt.Printf("  %s  %s\n", BrightBlack+fmt.Sprintf("[%d]", n)+Reset, msg)
}

// PrintInfo prints a generic informational line with a bullet.
func PrintInfo(msg string) {
	fmt.Printf("  %s  %s\n", BrightBlue+"›"+Reset, msg)
}
