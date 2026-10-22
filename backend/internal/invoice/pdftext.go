// Package invoice reads grocery invoices (Zepto, Blinkit, and similar GST tax invoices)
// from PDFs into an order total and line items.
package invoice

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Word is a run of glyphs with its horizontal extent.
type Word struct {
	X    float64 `json:"x"`
	X2   float64 `json:"x2"`
	Text string  `json:"t"`
}

// Line is the words sharing a baseline, left to right.
type Line struct {
	Y     float64 `json:"y"`
	Words []Word  `json:"w"`
}

type Page struct {
	Lines []Line `json:"lines"` // top to bottom
}

// ReadPDF extracts positioned words from every page. Invoice PDFs place glyphs
// individually, so words and table cells are rebuilt from coordinates.
func ReadPDF(data []byte) (pages []Page, err error) {
	defer func() {
		if r := recover(); r != nil { // the PDF library panics on some malformed files
			err = fmt.Errorf("couldn't read this PDF: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("couldn't read this PDF: %w", err)
	}
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		pages = append(pages, buildPage(p.Content().Text))
	}
	if len(pages) == 0 {
		return nil, errors.New("this PDF has no readable text (is it a scanned image?)")
	}
	return pages, nil
}

func buildPage(glyphs []pdf.Text) Page {
	sort.SliceStable(glyphs, func(a, b int) bool {
		if math.Abs(glyphs[a].Y-glyphs[b].Y) > 2 {
			return glyphs[a].Y > glyphs[b].Y
		}
		return glyphs[a].X < glyphs[b].X
	})
	var page Page
	for i := 0; i < len(glyphs); {
		j := i
		for j < len(glyphs) && math.Abs(glyphs[j].Y-glyphs[i].Y) <= 2 {
			j++
		}
		page.Lines = append(page.Lines, buildLine(glyphs[i:j]))
		i = j
	}
	return page
}

func isSpace(s string) bool { return s == " " || s == "\t" || s == "\u00a0" }

// relWidth approximates a glyph's width relative to an average character, since these
// PDFs report zero advance widths.
func relWidth(s string) float64 {
	switch {
	case strings.ContainsAny(s, "il.,:;'|!()[]/-"):
		return 0.55
	case strings.ContainsAny(s, "mwMW@%"):
		return 1.6
	case s >= "A" && s <= "Z":
		return 1.25
	}
	return 1.0
}

// buildLine groups glyphs into words: a space glyph ends a word, and so does a gap
// much larger than this line's typical character advance (table column boundaries).
func buildLine(gs []pdf.Text) Line {
	var adv []float64
	for k := 1; k < len(gs); k++ {
		if d := gs[k].X - gs[k-1].X; d > 0 && !isSpace(gs[k].S) && !isSpace(gs[k-1].S) {
			adv = append(adv, d/relWidth(gs[k-1].S))
		}
	}
	m := 5.0
	if len(adv) > 0 {
		sort.Float64s(adv)
		m = adv[len(adv)/2]
	}
	line := Line{Y: gs[0].Y}
	var word *Word
	flush := func() {
		if word != nil && strings.TrimSpace(word.Text) != "" {
			word.Text = strings.TrimSpace(word.Text)
			line.Words = append(line.Words, *word)
		}
		word = nil
	}
	var prev *pdf.Text
	for k := range gs {
		g := gs[k]
		if isSpace(g.S) {
			flush()
			prev = nil
			continue
		}
		if word != nil && prev != nil && g.X-prev.X > m*relWidth(prev.S)+m*1.1 {
			flush()
		}
		if word == nil {
			word = &Word{X: g.X}
		}
		word.Text += g.S
		word.X2 = g.X + m*relWidth(g.S)
		prev = &gs[k]
	}
	flush()
	return line
}

// Text returns a line as plain text with single spaces.
func (l Line) Text() string {
	parts := make([]string, len(l.Words))
	for i, w := range l.Words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}
