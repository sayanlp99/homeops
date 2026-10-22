package statement

import (
	"regexp"
	"strconv"
	"strings"
)

// HDFC (and a few other banks) offer a fixed-width text statement: a dashed rule marks
// the column spans, long narrations wrap onto continuation lines, and the header block
// repeats on every page. A summary at the end carries debit/credit totals and counts,
// which we use to prove nothing was dropped.

var (
	ruleRe     = regexp.MustCompile(`^-{4,}(\s+-{4,}){3,}\s*$`)
	rowDateRe  = regexp.MustCompile(`^\d{2}/\d{2}/\d{2,4}\s`)
	numberRe   = regexp.MustCompile(`[0-9][0-9,]*\.[0-9]{2}|\b[0-9]+\b`)
	summaryHdr = regexp.MustCompile(`(?i)statement summary`)
)

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

type span struct{ start, end int }

func spansOf(rule string) []span {
	var out []span
	in := false
	for i, c := range rule {
		if c == '-' && !in {
			out = append(out, span{start: i})
			in = true
		} else if c != '-' && in {
			out[len(out)-1].end = i
			in = false
		}
	}
	if in {
		out[len(out)-1].end = len(rule)
	}
	return out
}

func slice(line string, s span, next int) string {
	if s.start >= len(line) {
		return ""
	}
	end := next
	if end > len(line) || end <= 0 {
		end = len(line)
	}
	return strings.TrimSpace(line[s.start:end])
}

// isFixedWidth reports whether text looks like a dashed-rule fixed-width statement.
func isFixedWidth(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if ruleRe.MatchString(strings.TrimRight(l, "\r ")) {
			return true
		}
	}
	return false
}

func readFixedWidth(text string) (Table, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	var spans []span
	var header []string
	var t Table
	narr := -1
	lastChunk := map[int]int{} // printed width of each row's latest narration line
	for i := 0; i < len(lines); i++ {
		line := expandTabs(strings.TrimRight(lines[i], " "))
		if strings.HasPrefix(strings.TrimSpace(line), "****") {
			break // end of transactions
		}
		if ruleRe.MatchString(line) {
			if spans == nil {
				spans = spansOf(line)
				if i+1 < len(lines) {
					hl := expandTabs(lines[i+1])
					for j, s := range spans {
						next := len(hl)
						if j+1 < len(spans) {
							next = spans[j+1].start
						}
						header = append(header, slice(hl, s, next))
						if strings.Contains(strings.ToLower(header[j]), "narration") || strings.Contains(strings.ToLower(header[j]), "description") {
							narr = j
						}
					}
					t = append(t, header)
				}
			}
			i++ // skip the header line (repeats on each page) and the rule below it
			continue
		}
		if spans == nil || strings.TrimSpace(line) == "" {
			continue
		}
		cells := make([]string, len(spans))
		for j, s := range spans {
			next := len(line)
			if j+1 < len(spans) {
				next = spans[j+1].start
			}
			cells[j] = slice(line, s, next)
		}
		switch {
		case rowDateRe.MatchString(line):
			t = append(t, cells)
			lastChunk[len(t)-1] = chunkLen(line, spans[narr])
		case len(t) > 1 && narr >= 0 && cells[0] == "" && cells[narr] != "" && onlyNarration(cells, narr):
			// Continuation of the previous narration. The bank wraps at a fixed width,
			// mid-word, so a full-width chunk continues without a space.
			prev := t[len(t)-1]
			width := spans[narr].end - spans[narr].start
			if lastChunk[len(t)-1] == width {
				prev[narr] += cells[narr]
			} else {
				prev[narr] += " " + cells[narr]
			}
			lastChunk[len(t)-1] = chunkLen(line, spans[narr])
		}
	}
	return t, nil
}

// chunkLen is how much of the narration column a line actually filled.
func chunkLen(line string, s span) int {
	if s.start >= len(line) {
		return 0
	}
	end := min(s.end, len(line))
	return len(strings.TrimRight(line[s.start:end], " "))
}

func onlyNarration(cells []string, narr int) bool {
	for j, c := range cells {
		if j != narr && c != "" {
			return false
		}
	}
	return true
}

// Summary is the bank's own totals, used to check an import is complete.
type Summary struct {
	Debits  int64 `json:"debits"`
	Credits int64 `json:"credits"`
	DrCount int   `json:"drCount"`
	CrCount int   `json:"crCount"`
}

// ReadSummary extracts the HDFC-style statement summary, if present.
func ReadSummary(data []byte) *Summary {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n")
	for i, l := range lines {
		if !summaryHdr.MatchString(l) {
			continue
		}
		var s Summary
		found := 0
		for j := i + 1; j < len(lines) && j < i+12; j++ {
			low := strings.ToLower(lines[j])
			if strings.Contains(low, "debits") && strings.Contains(low, "credits") && j+1 < len(lines) {
				nums := numberRe.FindAllString(lines[j+1], -1)
				if len(nums) >= 4 {
					d, _ := ParseAmount(nums[1])
					c, _ := ParseAmount(nums[2])
					s.Debits, s.Credits = d.value, c.value
					found++
				}
			}
			if strings.Contains(low, "dr count") && j+1 < len(lines) {
				nums := numberRe.FindAllString(lines[j+1], -1)
				if len(nums) >= 2 {
					s.DrCount, _ = strconv.Atoi(nums[0])
					s.CrCount, _ = strconv.Atoi(nums[1])
					found++
				}
			}
		}
		if found == 2 {
			return &s
		}
	}
	return nil
}

// Check compares extracted rows with the bank's summary; it returns a problem description,
// or "" when every debit and credit is accounted for.
func (s *Summary) Check(rows []Row) string {
	var d, c int64
	var dn, cn int
	for _, r := range rows {
		if r.Debit {
			d += r.Amount
			dn++
		} else {
			c += r.Amount
			cn++
		}
	}
	if d == s.Debits && c == s.Credits && dn == s.DrCount && cn == s.CrCount {
		return ""
	}
	return "Totals don't match the bank's summary — some rows may be missing. Please check before importing."
}
