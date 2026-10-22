package invoice

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"homeops/internal/statement"
)

type Item struct {
	Name     string   `json:"name"`
	Qty      float64  `json:"qty"`
	Amount   int64    `json:"amount"` // paise, line total incl. tax
	Fee      bool     `json:"fee"`    // delivery/handling/platform charges, not goods
	PackQty  *float64 `json:"packQty"`
	PackUnit string   `json:"packUnit"` // "g", "ml", "l", "kg", "pcs"
}

type Invoice struct {
	Merchant  string   `json:"merchant"`
	OrderID   string   `json:"orderId"`
	Date      string   `json:"date"` // YYYY-MM-DD, "" if not found
	Total     int64    `json:"total"`
	ItemTotal int64    `json:"itemTotal"`
	Items     []Item   `json:"items"`
	Warnings  []string `json:"warnings"`
}

var (
	intRe       = regexp.MustCompile(`^\d{1,3}$`)
	moneyRe     = regexp.MustCompile(`^[+-]?\d[\d,]*\.\d{2}$`)
	numRe       = regexp.MustCompile(`^[+-]?\d[\d,]*(\.\d+)?%?$`)
	hsnRe       = regexp.MustCompile(`\s*\(HSN-?\s*[0-9]*\s*\)?`)
	feeRe       = regexp.MustCompile(`(?i)(delivery|handling|surge|convenience|platform|packaging|packing|small cart|late night|rain|service)\b.*\b(fee|charges?)\b|delivery and other charges|^tip\b`)
	packRe      = regexp.MustCompile(`(?i)\(?\s*(\d+(?:\.\d+)?)\s*(kg|g|gm|gms|ml|l|ltr|litre|pcs|pc|pieces|piece|pack|packs|n)\s*\)?\s*$`)
	orderRe     = regexp.MustCompile(`(?i)order\s*(?:no|id|number)\.?\s*:?\s*([A-Z0-9][A-Z0-9-]{4,})`)
	dateRe      = regexp.MustCompile(`(?i)\b(?:invoice\s*date|order\s*date|date)\s*:?\s*(\d{4}-\d{2}-\d{2}|\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}|\d{1,2}[- ][A-Za-z]{3,9}[- ,]+\d{2,4})`)
	grandRe     = regexp.MustCompile(`(?i)^(invoice value|grand total|total amount|order total|amount payable|net payable|total payable|bill total)\b`)
	packagingRe = regexp.MustCompile(`(?i)\s*\((pack|box|can|bottle|pet bottle|glass bottle|pouch|jar|packet|tin|tetra pack|tetrapack|carton|bag|combo|sachet|tub|cup|loose)\)\s*$`)
	makerRe     = regexp.MustCompile(`(?i)\b(company|ltd|limited|pvt|private|industries|foods|india)\b`)
	merchantMap = []struct{ key, name string }{
		{"blinkit", "Blinkit"}, {"zepto", "Zepto"}, {"instamart", "Swiggy Instamart"}, {"bigbasket", "BigBasket"}, {"bbnow", "BigBasket"},
		{"jiomart", "JioMart"}, {"dmart", "DMart"}, {"flipkart", "Flipkart"}, {"amazon", "Amazon"}, {"swiggy", "Swiggy"}, {"zomato", "Zomato"},
	}
)

type column struct{ x float64 }

// Parse reads items and totals from positioned words.
func Parse(pages []Page) *Invoice {
	inv := &Invoice{Items: []Item{}}
	var all strings.Builder
	orders := map[string]bool{}
	var grand, pageTotals int64
	for _, p := range pages {
		for _, l := range p.Lines {
			all.WriteString(l.Text())
			all.WriteByte('\n')
		}
		items, total, value := parsePage(p)
		inv.Items = append(inv.Items, items...)
		pageTotals += total
		grand += value
	}
	text := all.String()
	low := strings.ToLower(text)
	for _, m := range merchantMap {
		if strings.Contains(low, m.key) {
			inv.Merchant = m.name
			break
		}
	}
	if inv.Merchant == "" {
		inv.Merchant = sellerName(text)
	}
	for _, m := range orderRe.FindAllStringSubmatch(text, -1) {
		orders[m[1]] = true
	}
	ids := make([]string, 0, len(orders))
	for id := range orders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	inv.OrderID = strings.Join(ids, ", ")
	if m := dateRe.FindStringSubmatch(text); m != nil {
		if d, ok := statement.ParseDate(strings.TrimSpace(m[1])); ok {
			inv.Date = d.Format(time.DateOnly)
		}
	}
	for _, it := range inv.Items {
		inv.ItemTotal += it.Amount
	}
	switch {
	case grand > 0:
		inv.Total = grand
	case pageTotals > 0:
		inv.Total = pageTotals
	default:
		inv.Total = inv.ItemTotal
	}
	if len(inv.Items) == 0 {
		inv.Warnings = append(inv.Warnings, "No line items were recognised; only the total was read.")
	} else if d := inv.Total - inv.ItemTotal; d > 200 || d < -200 {
		inv.Warnings = append(inv.Warnings, "The items don't add up to the invoice total — some lines may be missing.")
	}
	if inv.Total == 0 {
		inv.Warnings = append(inv.Warnings, "Couldn't find the invoice total.")
	}
	return inv
}

func sellerName(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if i := strings.Index(strings.ToLower(l), "seller name"); i >= 0 {
			return strings.TrimSpace(strings.TrimLeft(l[i+len("seller name"):], ": "))
		}
	}
	return ""
}

func parseNum(s string) (float64, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.ReplaceAll(s, ",", ""), "+"), "%")
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func paise(s string) int64 {
	f, _ := parseNum(s)
	return int64(math.Round(f * 100))
}

// parsePage returns the page's items, its "Total" row amount and any grand total
// ("Invoice Value") printed on it.
func parsePage(p Page) (items []Item, pageTotal, grand int64) {
	// 1. Header: the line with "Description", plus neighbours within ~25pt.
	hi := -1
	for i, l := range p.Lines {
		for _, w := range l.Words {
			if strings.EqualFold(w.Text, "Description") {
				hi = i
				break
			}
		}
		if hi >= 0 {
			break
		}
	}
	if hi < 0 {
		return nil, 0, findGrand(p.Lines)
	}
	// Header rows are stacked tightly; the table body starts after a larger gap.
	top, bottom := hi, hi
	for top > 0 && p.Lines[top-1].Y-p.Lines[top].Y <= 10 {
		top--
	}
	for bottom+1 < len(p.Lines) && p.Lines[bottom].Y-p.Lines[bottom+1].Y <= 10 {
		bottom++
	}
	hy := p.Lines[hi].Y
	var header []Word
	for _, l := range p.Lines[top : bottom+1] {
		header = append(header, l.Words...)
	}
	descStart := math.Inf(1)
	for _, w := range header {
		if t := strings.ToLower(w.Text); t == "item" || t == "description" {
			descStart = math.Min(descStart, w.X)
		}
	}
	descEnd := math.Inf(1)
	var qtyX, totalX float64 = -1, -1
	for _, w := range header {
		t := strings.ToLower(strings.TrimSuffix(w.Text, "."))
		if w.X > descStart+15 && t != "item" && t != "&" && t != "description" {
			descEnd = math.Min(descEnd, w.X)
		}
		if t == "qty" || t == "quantity" {
			qtyX = w.X
		}
		if t == "total" && w.X > totalX {
			totalX = w.X
		}
	}
	if math.IsInf(descEnd, 1) || totalX < 0 {
		return nil, 0, findGrand(p.Lines)
	}
	headerBottom := p.Lines[bottom].Y - 0.5
	_ = hy

	// 2. Anchor rows: serial number (or "-" for Blinkit charge rows) left of the
	// description, with amounts to its right. The table ends at the first "Total" line.
	type anchor struct {
		idx    int
		y      float64
		charge bool // "-" row: its text is entirely on its own line
	}
	var anchors []anchor
	end := len(p.Lines)
	for i, l := range p.Lines {
		if l.Y > headerBottom || len(l.Words) == 0 {
			continue
		}
		first := l.Words[0]
		if isTotalLine(l) {
			end = i
			if n := lastMoney(l); n != "" {
				pageTotal = paise(n)
			}
			break
		}
		if first.X >= descStart || !(intRe.MatchString(first.Text) || first.Text == "-") {
			continue
		}
		nums := 0
		for _, w := range l.Words {
			if w.X >= descEnd-2 && numRe.MatchString(w.Text) {
				nums++
			}
		}
		if nums >= 3 {
			anchors = append(anchors, anchor{i, l.Y, first.Text == "-"})
		}
	}
	grand = findGrand(p.Lines[end:])
	if len(anchors) == 0 {
		return nil, pageTotal, grand
	}

	// 3. Description lines inside the table, grouped into blocks by vertical gaps.
	type dline struct {
		y    float64
		text string
	}
	isAnchor := map[int]anchor{}
	for _, a := range anchors {
		isAnchor[a.idx] = a
	}
	var dls []dline
	for i := 0; i < end; i++ {
		l := p.Lines[i]
		if l.Y > headerBottom {
			continue
		}
		if a, ok := isAnchor[i]; ok && a.charge {
			continue
		}
		var parts []string
		for _, w := range l.Words {
			if w.X >= descStart-6 && w.X < descEnd-2 {
				parts = append(parts, w.Text)
			}
		}
		if len(parts) > 0 {
			dls = append(dls, dline{l.Y, strings.Join(parts, " ")})
		}
	}
	var gaps []float64
	for k := 1; k < len(dls); k++ {
		gaps = append(gaps, dls[k-1].y-dls[k].y)
	}
	spacing := 10.0
	if len(gaps) > 0 {
		s := append([]float64(nil), gaps...)
		sort.Float64s(s)
		spacing = s[len(s)/4] // typical line spacing within one description
	}
	type block struct {
		top, bottom float64
		lines       []string
	}
	var blocks []block
	for k, d := range dls {
		if k == 0 || dls[k-1].y-d.y > spacing*1.25 {
			blocks = append(blocks, block{top: d.y, bottom: d.y})
		}
		b := &blocks[len(blocks)-1]
		b.bottom = d.y
		b.lines = append(b.lines, d.text)
	}
	names := map[int][]string{}
	for _, b := range blocks {
		best, bestD := -1, math.Inf(1)
		for ai, a := range anchors {
			if a.charge {
				continue
			}
			var d float64
			if a.y <= b.top+2 && a.y >= b.bottom-2 {
				d = 0
			} else {
				d = math.Min(math.Abs(a.y-b.top), math.Abs(a.y-b.bottom))
			}
			if d < bestD {
				best, bestD = ai, d
			}
		}
		if best >= 0 {
			names[best] = append(names[best], b.lines...)
		}
	}

	// 4. Build items from each anchor's amounts.
	for ai, a := range anchors {
		l := p.Lines[a.idx]
		var qty float64 = 1
		var total string
		qd, td := math.Inf(1), math.Inf(1)
		for _, w := range l.Words {
			if w.X < descEnd-2 || !numRe.MatchString(w.Text) {
				continue
			}
			if qtyX >= 0 && math.Abs(w.X-qtyX) < qd && math.Abs(w.X-qtyX) < 25 {
				if v, ok := parseNum(w.Text); ok && !strings.Contains(w.Text, "%") {
					qty, qd = v, math.Abs(w.X-qtyX)
				}
			}
			if math.Abs(w.X-totalX) < td && moneyRe.MatchString(w.Text) {
				total, td = w.Text, math.Abs(w.X-totalX)
			}
		}
		if total == "" {
			continue
		}
		var name string
		if a.charge {
			var parts []string
			for _, w := range l.Words {
				if w.X >= descStart-6 && w.X < descEnd-2 {
					parts = append(parts, w.Text)
				}
			}
			name = strings.Join(parts, " ")
		} else {
			name = joinLines(names[ai])
		}
		it := Item{Qty: qty, Amount: paise(total)}
		it.Name, it.PackQty, it.PackUnit = cleanName(name)
		it.Fee = a.charge || feeRe.MatchString(it.Name)
		if it.Name == "" {
			it.Name = "Item"
		}
		items = append(items, it)
	}
	return items, pageTotal, grand
}

func isTotalLine(l Line) bool {
	t := strings.ToLower(l.Words[0].Text)
	if t == "total" || (t == "item" && len(l.Words) > 1 && strings.EqualFold(l.Words[1].Text, "total")) {
		return true
	}
	// Blinkit puts "Total" a little right of the serial column.
	for _, w := range l.Words[:min(2, len(l.Words))] {
		if strings.EqualFold(w.Text, "total") {
			return true
		}
	}
	return false
}

func lastMoney(l Line) string {
	for k := len(l.Words) - 1; k >= 0; k-- {
		if moneyRe.MatchString(l.Words[k].Text) {
			return l.Words[k].Text
		}
	}
	return ""
}

func findGrand(lines []Line) int64 {
	var g int64
	for _, l := range lines {
		if grandRe.MatchString(l.Text()) {
			if n := lastMoney(l); n != "" {
				g = paise(n)
			}
		}
	}
	return g
}

// joinLines rebuilds a wrapped description: "Lemon-" + "Lime" → "Lemon-Lime".
func joinLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte(' ')
		}
		b.WriteString(l)
	}
	return b.String()
}

// cleanName drops HSN codes and manufacturer segments, and pulls out the pack size:
// "Sprite | Lemon-Lime Soft Drink | The Coca-Cola Company 1 pc (2.25 L)" →
// "Sprite Lemon-Lime Soft Drink", 2.25 l.
func cleanName(s string) (string, *float64, string) {
	s = strings.Join(strings.Fields(hsnRe.ReplaceAllString(s, "")), " ")
	// Blinkit appends the packaging type: "Soft Drink(PET Bottle)", "Eggs (6 pcs)(Pack)".
	for {
		t := packagingRe.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = strings.TrimSpace(t)
	}
	segs := strings.Split(s, "|")
	var keep []string
	var size string
	for i, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if m := packRe.FindStringSubmatchIndex(seg); m != nil && i == len(segs)-1 {
			size = seg[m[0]:]
			seg = strings.TrimSpace(seg[:m[0]])
			// "... Company 1 pc" — trailing count before the bracketed size.
			seg = strings.TrimSpace(regexp.MustCompile(`(?i)\s*\d+\s*(pc|pcs|pack|packs|piece|pieces|n)?\s*$`).ReplaceAllString(seg, ""))
		}
		if i > 0 && i == len(segs)-1 && makerRe.MatchString(seg) && len(segs) > 2 {
			continue // manufacturer line
		}
		if seg != "" {
			keep = append(keep, seg)
		}
	}
	name := strings.Join(keep, " ")
	if size == "" {
		return name, nil, ""
	}
	m := packRe.FindStringSubmatch(size)
	q, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return name, nil, ""
	}
	unit := strings.ToLower(m[2])
	switch unit {
	case "gm", "gms":
		unit = "g"
	case "ltr", "litre":
		unit = "l"
	case "pc", "pieces", "piece", "pack", "packs", "n":
		unit = "pcs"
	}
	return name, &q, unit
}
