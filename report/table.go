package main

import (
	"strings"
	"unicode/utf8"
)

// markdownTable is go-websocket-benchmark's markdown table: padded so that it
// lines up in the console as well as rendering as Markdown, every cell
// centred but the first column's, and a cell measured by its characters
// rather than its bytes, so that the [↓1] of a rank column's title, three
// bytes for one character, does not put its column out of line.
func markdownTable(title []string, rows [][]string) string {
	return markdownTableAligned(title, rows, false)
}

// markdownTableAligned is markdownTable, or with left set, a table whose
// every cell is padded on the right, so that it is left-aligned in the
// console too.
func markdownTableAligned(title []string, rows [][]string, left bool) string {
	width := utf8.RuneCountInString
	columnNum := len(title)
	maxLen := make([]int, columnNum)
	for i, v := range title {
		maxLen[i] = width(v)
	}

	all := make([][]string, 0, len(rows)+1)
	separator := make([]string, columnNum)
	for i := range separator {
		separator[i] = "---"
	}
	all = append(all, separator)
	all = append(all, rows...)
	for _, row := range all {
		for j, cell := range row {
			if j < columnNum && width(cell) > maxLen[j] {
				maxLen[j] = width(cell)
			}
		}
	}
	for i := range maxLen {
		maxLen[i] += 2
	}

	pad := padCell
	if left {
		pad = padLeft
	}
	var b strings.Builder
	b.WriteString("|")
	titleLeftPaddingIdx := 0
	for i, v := range title {
		aligned := pad(v, maxLen[i], false, 0)
		if i == 0 {
			// The last character that is not a space.
			for k, r := range []rune(aligned) {
				if r != ' ' {
					titleLeftPaddingIdx = k
				}
			}
		}
		b.WriteString(aligned)
		b.WriteString("|")
	}
	b.WriteString("\n")
	for _, row := range all {
		b.WriteString("|")
		for j := 0; j < columnNum; j++ {
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			b.WriteString(pad(cell, maxLen[j], j == 0, titleLeftPaddingIdx))
			b.WriteString("|")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// padCell pushes the first column right no further than its title and pads
// it on the right, and centres every other cell.
func padCell(s string, maxLen int, isFirst bool, titleLeftPaddingIdx int) string {
	paddingLen := maxLen - utf8.RuneCountInString(s)
	if paddingLen <= 0 {
		return s
	}
	if isFirst {
		for i := 0; i < paddingLen/2 && i < titleLeftPaddingIdx; i++ {
			s = " " + s
			paddingLen--
		}
		return s + strings.Repeat(" ", paddingLen)
	}
	half := paddingLen / 2
	return strings.Repeat(" ", half) + s + strings.Repeat(" ", paddingLen-half)
}

// padLeft left-aligns a cell: one space before it, the rest after.
func padLeft(s string, maxLen int, _ bool, _ int) string {
	paddingLen := maxLen - utf8.RuneCountInString(s)
	if paddingLen <= 0 {
		return s
	}
	return " " + s + strings.Repeat(" ", paddingLen-1)
}
