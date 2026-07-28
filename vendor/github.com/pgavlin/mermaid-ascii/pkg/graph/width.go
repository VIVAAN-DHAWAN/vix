package graph

import (
	"strings"

	"github.com/rivo/uniseg"
)

// This file makes the graph renderer display-width aware. The drawing grid is
// one cell per terminal column, but Unicode text is not one column per byte (or
// even per rune): emoji and East-Asian glyphs occupy two columns, and combining
// marks / ZWJ sequences occupy zero extra columns. Measuring with byte length
// and placing text byte-by-byte therefore both mangles multibyte runes and
// misaligns boxes. The helpers below measure and place text by grapheme cluster.

// dispWidth returns the terminal display width of s, counting grapheme clusters
// so a ZWJ emoji sequence counts once and wide (emoji / East-Asian) clusters
// count as two columns.
func dispWidth(s string) int {
	return uniseg.StringWidth(s)
}

// textCell is a single display cell: the grapheme-cluster string it holds and
// its terminal width (1 or 2 columns).
type textCell struct {
	s string
	w int
}

// splitCells segments s into display cells (grapheme clusters). Zero-width
// clusters (e.g. combining marks) are folded into the preceding cell so they
// never consume a grid column on their own.
func splitCells(s string) []textCell {
	var cells []textCell
	state := -1
	for len(s) > 0 {
		var cluster string
		var w int
		cluster, s, w, state = uniseg.FirstGraphemeClusterInString(s, state)
		if w == 0 {
			if len(cells) > 0 {
				cells[len(cells)-1].s += cluster
				continue
			}
			w = 1
		}
		cells = append(cells, textCell{cluster, w})
	}
	return cells
}

// cellsWidth returns the total column width of a cell slice.
func cellsWidth(cells []textCell) int {
	n := 0
	for _, c := range cells {
		n += c.w
	}
	return n
}

// cellsString reassembles a cell slice back into a string.
func cellsString(cells []textCell) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteString(c.s)
	}
	return b.String()
}

// truncateCells returns the longest prefix of s that fits within maxCols columns.
func truncateCells(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	col := 0
	var b strings.Builder
	for _, c := range splitCells(s) {
		if col+c.w > maxCols {
			break
		}
		b.WriteString(c.s)
		col += c.w
	}
	return b.String()
}

// putCells writes text into d starting at column x, row y, advancing one grid
// column per display column. A double-width cluster occupies two grid cells: the
// cluster followed by an empty continuation cell (""), so that downstream column
// math and drawingToString stay aligned (the cluster renders as two terminal
// columns while contributing two grid cells). colorHex (may be "") colorizes each
// cell for the given styleType. It returns the number of columns written.
func putCells(d *drawing, x, y int, text, colorHex, styleType string) int {
	total := dispWidth(text)
	if total > 0 {
		d.increaseSize(x+total, y)
	}
	col := 0
	for _, c := range splitCells(text) {
		cx := x + col
		if cx >= 0 && cx < len(*d) && y >= 0 && y < len((*d)[0]) {
			(*d)[cx][y] = wrapTextInColor(c.s, colorHex, styleType)
			for k := 1; k < c.w; k++ {
				if cx+k < len(*d) {
					(*d)[cx+k][y] = ""
				}
			}
		}
		col += c.w
	}
	return col
}
