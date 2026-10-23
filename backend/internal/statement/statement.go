// Package statement reads bank and credit-card statement exports (CSV, XLSX, and the
// HTML-in-disguise ".xls" many Indian banks serve) into transactions.
package statement

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// Table is a sheet of cells as text.
type Table [][]string

// Read detects the file format from its content (not its name) and returns all rows.
func Read(data []byte) (Table, error) {
	trimmed := bytes.TrimLeft(data, "\xef\xbb\xbf \t\r\n")
	switch {
	case bytes.HasPrefix(trimmed, []byte("PK")):
		return readXLSX(data)
	case bytes.HasPrefix(trimmed, []byte{0xD0, 0xCF, 0x11, 0xE0}):
		return nil, errors.New("this is an old-style Excel file (.xls). Open it in Excel or Google Sheets and save as .xlsx or .csv")
	case bytes.HasPrefix(trimmed, []byte("<")):
		return readHTML(trimmed)
	case bytes.HasPrefix(trimmed, []byte("%PDF")):
		return nil, errors.New("PDF statements aren't supported yet — download the CSV, Excel or text version instead")
	case isFixedWidth(string(trimmed)):
		return readFixedWidth(string(trimmed))
	}
	return readCSV(trimmed)
}

func readCSV(data []byte) (Table, error) {
	// Some banks use tabs, semicolons or pipes; pick whichever splits the most.
	best, bestCols := Table(nil), 0
	for _, sep := range []rune{',', '\t', ';', '|'} {
		r := csv.NewReader(bytes.NewReader(data))
		r.Comma = sep
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		r.TrimLeadingSpace = sep != '\t' // with tabs it would swallow empty cells
		rows, err := r.ReadAll()
		if err != nil {
			continue
		}
		// Score by how many rows share the most common column count: commas inside
		// amounts ("1,234.00") shouldn't beat a consistent tab-separated layout.
		counts := map[int]int{}
		for _, row := range rows {
			if len(row) > 1 {
				counts[len(row)]++
			}
		}
		score := 0
		for cols, n := range counts {
			score = max(score, cols*n)
		}
		if score > bestCols {
			best, bestCols = rows, score
		}
	}
	if best == nil {
		return nil, errors.New("couldn't read this file as CSV")
	}
	return clean(best), nil
}

func readXLSX(data []byte) (Table, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("couldn't open Excel file: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("the Excel file has no sheets")
	}
	// Use the sheet with the most rows (some exports put a summary sheet first).
	var best Table
	for _, s := range sheets {
		rows, err := f.GetRows(s, excelize.Options{RawCellValue: false})
		if err == nil && len(rows) > len(best) {
			best = rows
		}
	}
	return clean(best), nil
}

var (
	trRe   = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRe = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	tagRe  = regexp.MustCompile(`<[^>]+>`)
)

func readHTML(data []byte) (Table, error) {
	var t Table
	for _, tr := range trRe.FindAllSubmatch(data, -1) {
		var row []string
		for _, td := range cellRe.FindAllSubmatch(tr[1], -1) {
			row = append(row, html.UnescapeString(tagRe.ReplaceAllString(string(td[1]), " ")))
		}
		if len(row) > 0 {
			t = append(t, row)
		}
	}
	if len(t) == 0 {
		return nil, errors.New("no table found in this file")
	}
	return clean(t), nil
}

func clean(t Table) Table {
	out := make(Table, 0, len(t))
	for _, row := range t {
		empty := true
		for i, c := range row {
			row[i] = strings.Join(strings.Fields(strings.ReplaceAll(c, " ", " ")), " ")
			if row[i] != "" {
				empty = false
			}
		}
		if !empty {
			out = append(out, row)
		}
	}
	return out
}

// ---- column mapping ----

// Mapping says which column holds what; -1 = absent. Columns are 0-based.
type Mapping struct {
	HeaderRow   int `json:"headerRow"`
	Date        int `json:"date"`
	Description int `json:"description"`
	Debit       int `json:"debit"`
	Credit      int `json:"credit"`
	Amount      int `json:"amount"` // single signed/flagged amount column
	DrCr        int `json:"drCr"`   // "Dr"/"Cr" indicator for Amount
	Ref         int `json:"ref"`
}

var synonyms = map[string][]string{
	"date":        {"txn date", "transaction date", "tran date", "trans date", "date", "posting date", "value date", "value dt"},
	"description": {"narration", "transaction remarks", "transaction details", "transaction description", "description", "particulars", "remarks", "details", "merchant"},
	"debit":       {"withdrawal amt", "withdrawal amount", "withdrawals", "withdrawal", "debit amount", "debit amt", "debit", "dr amount", "dr"},
	"credit":      {"deposit amt", "deposit amount", "deposits", "deposit", "credit amount", "credit amt", "credit", "cr amount", "cr"},
	"amount":      {"amount (inr)", "amount (in rs.)", "amount(in rs)", "transaction amount", "amount in inr", "amount", "amt"},
	"drcr":        {"dr/cr", "cr/dr", "debit/credit", "type", "transaction type", "dr / cr", "cr / dr"},
	"ref":         {"chq./ref.no.", "chq/ref no", "ref no./cheque no.", "ref no", "reference no", "reference", "cheque number", "cheque no", "chq no", "chqno", "utr"},
}

func norm(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.NewReplacer("(inr )", "(inr)", "  ", " ", "₹", "").Replace(h)
	return strings.TrimSpace(h)
}

// matchHeader scores how well a cell names a field: exact synonym beats prefix.
func matchHeader(cell, field string) int {
	c := norm(cell)
	for i, s := range synonyms[field] {
		weight := 100 - i // earlier synonyms are more specific
		if c == s {
			return 1000 + weight
		}
		if strings.HasPrefix(c, s+" ") || strings.HasPrefix(c, s+".") || strings.HasPrefix(c, s+"(") {
			return 500 + weight
		}
	}
	return 0
}

// Detect finds the header row (banks put account details above it) and maps columns.
func Detect(t Table) (Mapping, error) {
	bestScore, best := 0, Mapping{}
	for r := 0; r < len(t) && r < 60; r++ {
		m := Mapping{HeaderRow: r, Date: -1, Description: -1, Debit: -1, Credit: -1, Amount: -1, DrCr: -1, Ref: -1}
		used := map[int]bool{}
		score := 0
		for _, f := range []struct {
			name string
			dst  *int
		}{{"date", &m.Date}, {"description", &m.Description}, {"debit", &m.Debit}, {"credit", &m.Credit}, {"drcr", &m.DrCr}, {"amount", &m.Amount}, {"ref", &m.Ref}} {
			bi, bs := -1, 0
			for c, cell := range t[r] {
				if used[c] {
					continue
				}
				if s := matchHeader(cell, f.name); s > bs {
					bi, bs = c, s
				}
			}
			if bi >= 0 {
				*f.dst = bi
				used[bi] = true
				score += bs
			}
		}
		if m.Date < 0 || m.Description < 0 || (m.Amount < 0 && m.Debit < 0 && m.Credit < 0) {
			continue
		}
		if m.Debit >= 0 || m.Credit >= 0 {
			m.Amount, m.DrCr = -1, -1
		}
		if score > bestScore {
			bestScore, best = score, m
		}
	}
	if bestScore == 0 {
		return Mapping{HeaderRow: -1, Date: -1, Description: -1, Debit: -1, Credit: -1, Amount: -1, DrCr: -1, Ref: -1},
			errors.New("couldn't recognise the columns — pick them below")
	}
	return best, nil
}

// ---- rows ----

type Row struct {
	Line        int       `json:"line"` // 1-based row number in the file
	Date        time.Time `json:"-"`
	DateStr     string    `json:"date"`
	Description string    `json:"description"`
	Ref         string    `json:"ref"`
	Amount      int64     `json:"amount"` // paise, always positive
	Debit       bool      `json:"debit"`  // money out
	Payee       string    `json:"payee"`
}

// Extract converts data rows using the mapping. Rows that aren't transactions (totals,
// opening balance, blank amounts, footers) are skipped.
func Extract(t Table, m Mapping) ([]Row, []string) {
	var out []Row
	var warnings []string
	cell := func(row []string, i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return row[i]
	}
	for r := m.HeaderRow + 1; r < len(t); r++ {
		row := t[r]
		d, ok := ParseDate(cell(row, m.Date))
		if !ok {
			continue
		}
		desc := cell(row, m.Description)
		low := strings.ToLower(desc)
		if strings.Contains(low, "opening balance") || strings.Contains(low, "closing balance") || strings.HasPrefix(low, "total") {
			continue
		}
		var amt int64
		var debit bool
		if m.Debit >= 0 || m.Credit >= 0 {
			dv, dok := ParseAmount(cell(row, m.Debit))
			cv, cok := ParseAmount(cell(row, m.Credit))
			switch {
			case dok && dv.value != 0:
				amt, debit = abs(dv.value), true
			case cok && cv.value != 0:
				amt, debit = abs(cv.value), false
			default:
				continue
			}
		} else {
			v, ok := ParseAmount(cell(row, m.Amount))
			if !ok || v.value == 0 {
				continue
			}
			amt = abs(v.value)
			flag := strings.ToLower(cell(row, m.DrCr))
			switch {
			case strings.HasPrefix(flag, "cr") || strings.Contains(flag, "credit"):
				debit = false
			case strings.HasPrefix(flag, "dr") || strings.Contains(flag, "debit"):
				debit = true
			case v.flag != "":
				debit = v.flag == "dr"
			default:
				// Card statements list spends as positive; negative means refund/payment.
				debit = v.value > 0
			}
		}
		out = append(out, Row{Line: r + 1, Date: d, DateStr: d.Format(time.DateOnly), Description: desc, Ref: cell(row, m.Ref), Amount: amt, Debit: debit, Payee: Payee(desc)})
	}
	if len(out) == 0 {
		warnings = append(warnings, "No transactions found with this column mapping.")
	}
	return out, warnings
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---- parsing helpers ----

var dateLayouts = []string{
	"02/01/2006", "02/01/06", "2/1/2006", "2/1/06", "02-01-2006", "02-01-06", "2-1-2006",
	"02.01.2006", "02.01.06", "02-Jan-2006", "02-Jan-06", "2-Jan-2006", "02 Jan 2006", "2 Jan 2006",
	"02 Jan 06", "02-January-2006", "02 January 2006", "January 2, 2006", "Jan 2, 2006",
	"2006-01-02", "2006/01/02", "02/01/2006 15:04:05", "02/01/2006 15:04", "02-01-2006 15:04:05",
	"2006-01-02 15:04:05", "02 Jan 2006 15:04", "02-Jan-2006 15:04:05",
}

// ParseDate reads Indian day-first dates, ISO dates and Excel serial numbers.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 30000 && n < 80000 { // Excel serial
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)), true
	}
	s = strings.ReplaceAll(s, "Sept", "Sep")
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			if t.Year() < 2000 || t.Year() > 2100 {
				continue
			}
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	// Title-case month names: "05-OCT-2026".
	if t, ok := parseUpperMonth(s); ok {
		return t, true
	}
	return time.Time{}, false
}

func parseUpperMonth(s string) (time.Time, bool) {
	b := []rune(strings.ToLower(s))
	for i := range b {
		if unicode.IsLetter(b[i]) && (i == 0 || !unicode.IsLetter(b[i-1])) {
			b[i] = unicode.ToUpper(b[i])
		}
	}
	t := string(b)
	if t == s {
		return time.Time{}, false
	}
	for _, l := range dateLayouts {
		if d, err := time.Parse(l, t); err == nil && d.Year() >= 2000 {
			return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

type amount struct {
	value int64  // paise, signed
	flag  string // "dr" / "cr" when written in the cell
}

var amountRe = regexp.MustCompile(`^\(?-?[0-9][0-9,]*(\.[0-9]+)?\)?$`)

// ParseAmount handles "1,23,456.78", "₹ 450", "1,234.00 Cr", "(1,234.00)", "-200".
func ParseAmount(s string) (amount, bool) {
	s = strings.TrimSpace(strings.NewReplacer("₹", "", "Rs.", "", "Rs", "", "INR", "", " ", "").Replace(s))
	var a amount
	low := strings.ToLower(s)
	switch {
	case strings.HasSuffix(low, "cr"):
		a.flag, s = "cr", s[:len(s)-2]
	case strings.HasSuffix(low, "dr"):
		a.flag, s = "dr", s[:len(s)-2]
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || !amountRe.MatchString(s) {
		return a, false
	}
	neg := strings.HasPrefix(s, "-") || (strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"))
	s = strings.Trim(s, "()-")
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return a, false
	}
	a.value = int64(math.Round(f * 100))
	if neg {
		a.value = -a.value
	}
	return a, true
}

var (
	payeeSplit = regexp.MustCompile(`[/\-*|:]+`)
	noiseWords = map[string]bool{"upi": true, "dr": true, "cr": true, "imps": true, "neft": true, "rtgs": true, "pos": true, "ach": true,
		"nach": true, "ecs": true, "mb": true, "ib": true, "inb": true, "bil": true, "bill": true, "payment": true, "pay": true, "to": true,
		"from": true, "by": true, "transfer": true, "trf": true, "txn": true, "ref": true, "p2a": true, "p2m": true, "collect": true,
		"sent": true, "using": true, "paytm": false, "atm": true, "wdl": true, "atw": true, "nwd": true, "eaw": true, "vin": true, "vps": true,
		"mmt": true, "int": true, "fund": true, "na": true, "null": true, "others": true, "other": true, "ok": true}
	boilerplateRe = regexp.MustCompile(`^(payment from|paid via|sent using|payment to|upi payment|thank you|transfer to|transfer from)\b`)
	atmRe         = regexp.MustCompile(`(?i)^(nwd|atw|eaw|atm|cash wdl|atm wdl)[-/ ]`)
	bankCodeRe    = regexp.MustCompile(`(?i)^[a-z]{4}0[a-z0-9]{6}$`) // IFSC
	digitsRe      = regexp.MustCompile(`\d`)
)

// Payee pulls a merchant or person name out of a bank narration:
// "UPI-SWIGGY-swiggy@icici-ICIC0000103-123456789012-UPI" → "Swiggy",
// "UPI/DR/412345678901/RAMESH KUMAR/SBIN/ramesh@okaxis/Payment" → "Ramesh Kumar",
// "POS 416021XXXXXX1234 AMAZON PAY INDIA" → "Amazon Pay India".
func Payee(desc string) string {
	if atmRe.MatchString(desc) {
		return "ATM cash withdrawal"
	}
	var vpa string
	for _, tok := range payeeSplit.Split(desc, -1) {
		tok = strings.TrimSpace(tok)
		if tok == "" || bankCodeRe.MatchString(tok) {
			continue
		}
		if strings.Contains(tok, "@") {
			if vpa == "" {
				vpa = strings.SplitN(tok, "@", 2)[0]
			}
			continue
		}
		low := strings.ToLower(tok)
		if strings.Contains(low, "bank") || boilerplateRe.MatchString(low) {
			continue // the payer's/payee's bank or a default remark, not who was paid
		}
		// Drop leading noise words ("POS", "TO TRANSFER") and any reference numbers or
		// card masks; keep words like "Pay" in "Amazon Pay".
		var kept []string
		for _, w := range strings.Fields(tok) {
			lw := strings.ToLower(w)
			letters := 0
			for _, r := range w {
				if unicode.IsLetter(r) {
					letters++
				}
			}
			if letters == 0 || len(digitsRe.FindAllString(w, -1)) >= 2 || (len(kept) == 0 && noiseWords[lw]) {
				continue
			}
			kept = append(kept, w)
		}
		name := strings.Join(kept, " ")
		letters := 0
		for _, r := range name {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		// Short all-caps codes like "YESB" or "SBIN" are bank identifiers.
		if letters < 3 || (len(kept) == 1 && len(name) <= 4 && strings.ToUpper(name) == name) {
			continue
		}
		return titleCase(name)
	}
	if v := strings.Trim(digitsRe.ReplaceAllString(vpa, ""), ".-_"); len(v) >= 3 {
		return titleCase(v)
	}
	return titleCase(desc)
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	out := strings.Join(words, " ")
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
