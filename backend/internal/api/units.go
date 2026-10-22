package api

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Units are stored in canonical short form. Mass and volume convert within their
// dimension; everything else (pcs, bunch, packet…) is its own dimension.
type unitInfo struct {
	canon  string
	dim    string  // "mass", "volume" or the unit itself for counts
	toBase float64 // multiplier to grams / millilitres
}

var unitTable = map[string]unitInfo{}

func init() {
	add := func(canon, dim string, toBase float64, aliases ...string) {
		u := unitInfo{canon, dim, toBase}
		unitTable[canon] = u
		for _, a := range aliases {
			unitTable[a] = u
		}
	}
	add("g", "mass", 1, "gm", "gms", "gram", "grams", "gr", "grm")
	add("kg", "mass", 1000, "kgs", "kilo", "kilos", "kilogram", "kilograms")
	add("mg", "mass", 0.001, "milligram", "milligrams")
	add("oz", "mass", 28.35, "ounce", "ounces")
	add("lb", "mass", 453.6, "lbs", "pound", "pounds")
	add("ml", "volume", 1, "millilitre", "milliliter", "millilitres", "milliliters", "mls")
	add("l", "volume", 1000, "litre", "liter", "litres", "liters", "ltr", "ltrs", "lt")
	add("tsp", "volume", 5, "teaspoon", "teaspoons", "tsps", "t")
	add("tbsp", "volume", 15, "tablespoon", "tablespoons", "tbsps", "tbs", "tbl", "T")
	add("cup", "volume", 240, "cups", "c", "katori", "katoris", "glass", "glasses")
	add("fl oz", "volume", 29.57, "floz", "fluid ounce", "fluid ounces")
	for _, c := range [][]string{
		{"pcs", "pc", "piece", "pieces", "nos", "no", "number", "whole"},
		{"clove", "cloves"}, {"bunch", "bunches", "gaddi"}, {"packet", "packets", "pack", "packs", "pkt", "pkts"},
		{"can", "cans", "tin", "tins"}, {"pinch", "pinches"}, {"inch", "inches", "in"}, {"sprig", "sprigs"},
		{"slice", "slices"}, {"dozen", "dozens", "dz"}, {"bottle", "bottles"}, {"handful", "handfuls"},
		{"sheet", "sheets"}, {"stick", "sticks"}, {"leaf", "leaves"}, {"head", "heads"}, {"pod", "pods"},
	} {
		add(c[0], c[0], 1, c[1:]...)
	}
}

func lookupUnit(s string) (unitInfo, bool) {
	if u, ok := unitTable[s]; ok { // case-sensitive first: "T" = tbsp, "t" = tsp
		return u, true
	}
	u, ok := unitTable[strings.ToLower(strings.TrimSuffix(s, "."))]
	return u, ok
}

// normalizeUnit returns the canonical spelling, or the input lowercased if unknown.
func normalizeUnit(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if u, ok := lookupUnit(s); ok {
		return u.canon
	}
	return strings.ToLower(s)
}

func unitDim(unit string) string {
	if u, ok := lookupUnit(unit); ok {
		return u.dim
	}
	if unit == "" {
		return "pcs"
	}
	return unit
}

// convert expresses qty in `from` units as `to` units; ok is false across dimensions.
func convert(qty float64, from, to string) (float64, bool) {
	if from == to {
		return qty, true
	}
	if (from == "" && to == "pcs") || (from == "pcs" && to == "") {
		return qty, true
	}
	f, ok1 := lookupUnit(from)
	t, ok2 := lookupUnit(to)
	if !ok1 || !ok2 || f.dim != t.dim {
		return 0, false
	}
	return qty * f.toBase / t.toBase, true
}

// humanize picks a friendly unit for a summed amount: 1500 g → 1.5 kg, 3 tsp → 1 tbsp.
func humanize(qty float64, unit string) (float64, string) {
	switch unitDim(unit) {
	case "mass":
		g, _ := convert(qty, unit, "g")
		if g >= 1000 {
			return round2(g / 1000), "kg"
		}
		return math.Round(g), "g"
	case "volume":
		ml, _ := convert(qty, unit, "ml")
		switch {
		case unit == "tsp" || unit == "tbsp" || unit == "cup":
			// Cooking measures stay as-is: "8 cups poha", not "2 l poha".
			return round2(qty), unit
		case ml >= 1000:
			return round2(ml / 1000), "l"
		default:
			return math.Round(ml), "ml"
		}
	}
	return round2(qty), unit
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// ---- ingredient line parsing ----

type parsedLine struct {
	Quantity *float64 `json:"quantity"`
	Unit     string   `json:"unit"`
	Name     string   `json:"name"`
	Note     string   `json:"note"`
}

var (
	unicodeFractions = map[rune]string{'½': "1/2", '⅓': "1/3", '⅔': "2/3", '¼': "1/4", '¾': "3/4", '⅕': "1/5", '⅛': "1/8", '⅜': "3/8", '⅝': "5/8", '⅞': "7/8"}
	// Leading amount: "1", "1.5", "1/2", "1 1/2", "1-2", "2 to 3"; unicode fractions are pre-expanded.
	qtyRe       = regexp.MustCompile(`^(\d+/\d+|\d+(?:[.,]\d+)?(?:\s+\d+/\d+)?)(?:\s*(?:-|–|to)\s*(\d+/\d+|\d+(?:[.,]\d+)?))?\s*`)
	sizeRe      = regexp.MustCompile(`(?i)^(small|medium|large|big|heaped|level|heaping)\s+`)
	spaceRe     = regexp.MustCompile(`\s+`)
	toTasteRe   = regexp.MustCompile(`(?i),?\s*(to taste|as needed|as required|as per taste|optional)\s*$`)
	glued       = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)([a-zA-Z]+)\b`) // "200g", "1kg"
	articlesRe  = regexp.MustCompile(`(?i)^(of|a|an)\s+`)
)

func parseAmount(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	total := 0.0
	for _, part := range strings.Fields(s) {
		if n, d, found := strings.Cut(part, "/"); found {
			a, err1 := strconv.ParseFloat(n, 64)
			b, err2 := strconv.ParseFloat(d, 64)
			if err1 != nil || err2 != nil || b == 0 {
				return 0, false
			}
			total += a / b
		} else {
			v, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return 0, false
			}
			total += v
		}
	}
	return total, true
}

// parseIngredientLine turns "1½ cups basmati rice, washed" into {1.5 cup "basmati rice" "washed"}.
// Ranges ("2-3 green chillies") take the upper bound so the shopping list buys enough.
func parseIngredientLine(line string) parsedLine {
	s := strings.TrimSpace(line)
	s = strings.TrimLeft(s, "-•*▢□☐● \t")
	// Parenthetical asides become notes, kept verbatim:
	// "1 cup (200 g) toor dal ((split pigeon peas))".
	s, notes := extractParens(s)
	var b strings.Builder
	for i, r := range s {
		if f, ok := unicodeFractions[r]; ok {
			// "1½" → "1 1/2"
			if i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
				b.WriteByte(' ')
			}
			b.WriteString(f)
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()

	var out parsedLine
	if m := toTasteRe.FindStringSubmatch(s); m != nil {
		notes = append(notes, strings.ToLower(m[1]))
		s = strings.TrimSpace(s[:len(s)-len(m[0])])
	}
	if m := glued.FindStringSubmatch(s); m != nil {
		if _, ok := lookupUnit(m[2]); ok {
			s = m[1] + " " + m[2] + s[len(m[0]):]
		}
	}
	if m := qtyRe.FindStringSubmatch(s); m != nil && m[0] != "" {
		amt, ok := parseAmount(m[1])
		if m[2] != "" {
			if hi, ok2 := parseAmount(m[2]); ok2 {
				amt, ok = hi, true
			}
		}
		if ok {
			out.Quantity = &amt
			s = s[len(m[0]):]
		}
	}
	s = articlesRe.ReplaceAllString(strings.TrimSpace(s), "") // "a pinch of salt"

	if word, rest, _ := strings.Cut(s, " "); word != "" {
		if u, ok := lookupUnit(word); ok && (out.Quantity != nil || u.dim != "pcs") && rest != "" {
			out.Unit = u.canon
			s = rest
		} else if word2, rest2, found := strings.Cut(rest, " "); found {
			if u, ok := lookupUnit(word + " " + word2); ok { // "fl oz"
				out.Unit = u.canon
				s = rest2
			}
		}
	}
	s = articlesRe.ReplaceAllString(strings.TrimSpace(s), "")
	if m := sizeRe.FindStringSubmatch(s); m != nil { // "medium onion" → onion (medium)
		notes = append(notes, strings.ToLower(m[1]))
		s = s[len(m[0]):]
	}
	if name, note, found := strings.Cut(s, ","); found {
		s = name
		notes = append([]string{strings.TrimSpace(note)}, notes...)
	}
	out.Name = cleanName(s)
	out.Note = strings.Join(filterEmpty(notes), "; ")
	if out.Name == "" {
		out.Name = cleanName(line)
	}
	return out
}

// extractParens removes balanced (possibly nested) parenthetical groups, returning the
// remaining text and each group's contents with inner parentheses stripped.
func extractParens(s string) (string, []string) {
	var out, cur strings.Builder
	var notes []string
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			if depth == 0 {
				cur.Reset()
			}
			depth++
		case r == ')' && depth > 0:
			depth--
			if depth == 0 {
				if n := strings.TrimSpace(cur.String()); n != "" {
					notes = append(notes, n)
				}
				out.WriteByte(' ')
			}
		case depth > 0:
			cur.WriteRune(r)
		case r != ')':
			out.WriteRune(r)
		}
	}
	if depth > 0 { // unbalanced: keep what was inside as a note
		if n := strings.TrimSpace(cur.String()); n != "" {
			notes = append(notes, n)
		}
	}
	return spaceRe.ReplaceAllString(out.String(), " "), notes
}

func cleanName(s string) string {
	s = spaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.Trim(s, " .,;:-")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func filterEmpty(xs []string) []string {
	out := xs[:0]
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ---- aisles ----

var aisles = []string{
	"Vegetables", "Fruits & Nuts", "Dairy & Eggs", "Meat & Fish", "Grains & Flour", "Pulses & Lentils",
	"Spices & Masala", "Oils & Ghee", "Bakery", "Snacks & Packaged", "Beverages", "Frozen", "Household", "Other",
}

// Checked in order, so specific phrases ("chilli powder") win over generic ones ("chilli").
type aisleRule struct {
	aisle    string
	keywords []string
	re       *regexp.Regexp
}

var aisleRules = []aisleRule{
	{aisle: "Spices & Masala", keywords: []string{"powder", "masala", "turmeric", "haldi", "jeera", "cumin", "coriander seed", "dhania seed", "mustard seed", "rai", "fenugreek seed", "methi seed", "fennel", "saunf", "ajwain", "carom", "hing", "asafoetida", "cardamom", "elaichi", "clove", "laung", "cinnamon", "dalchini", "bay leaf", "tej patta", "peppercorn", "black pepper", "kasuri", "star anise", "nutmeg", "mace", "saffron", "kesar", "salt", "sugar", "jaggery", "gur", "chaat", "amchur", "kalonji", "nigella", "poppy", "sesame", "til", "red chilli", "kashmiri", "dry chilli", "dried chilli", "baking soda", "baking powder", "yeast", "vinegar", "soy sauce", "ketchup", "sauce"}},
	{aisle: "Oils & Ghee", keywords: []string{"oil", "ghee", "vanaspati", "dalda"}},
	{aisle: "Dairy & Eggs", keywords: []string{"milk", "curd", "dahi", "yogurt", "yoghurt", "paneer", "butter", "cheese", "cream", "malai", "khoya", "mawa", "buttermilk", "chaas", "egg"}},
	{aisle: "Meat & Fish", keywords: []string{"chicken", "mutton", "lamb", "goat", "fish", "prawn", "shrimp", "crab", "pork", "beef", "keema", "mince", "salmon", "rohu", "hilsa", "ilish", "pomfret", "surmai", "bekti", "tuna", "bacon", "sausage", "ham"}},
	{aisle: "Pulses & Lentils", keywords: []string{"dal", "daal", "lentil", "moong", "masoor", "toor", "arhar", "urad", "chana", "chickpea", "rajma", "kidney bean", "lobia", "black eyed", "matki", "kulthi", "soya chunk", "soybean"}},
	{aisle: "Grains & Flour", keywords: []string{"rice", "basmati", "atta", "flour", "maida", "besan", "suji", "sooji", "rava", "semolina", "poha", "oats", "millet", "ragi", "bajra", "jowar", "quinoa", "dalia", "vermicelli", "sevai", "pasta", "noodle", "macaroni", "spaghetti", "cornflour", "corn starch", "sabudana", "sago", "murmura", "puffed rice", "idli rava"}},
	{aisle: "Bakery", keywords: []string{"bread", "pav", "bun", "rusk", "toast", "croissant", "kulcha"}},
	{aisle: "Fruits & Nuts", keywords: []string{"apple", "banana", "mango", "orange", "papaya", "grape", "pomegranate", "anar", "guava", "watermelon", "melon", "pineapple", "chikoo", "sapota", "pear", "kiwi", "strawberry", "strawberries", "litchi", "lychee", "dates", "khajur", "raisin", "kishmish", "almond", "badam", "cashew", "kaju", "walnut", "akhrot", "pistachio", "pista", "fig", "anjeer", "coconut", "peanut", "groundnut", "makhana", "foxnut"}},
	{aisle: "Frozen", keywords: []string{"frozen", "ice cream"}},
	{aisle: "Beverages", keywords: []string{"tea", "chai patti", "coffee", "juice", "soda", "cola", "water bottle"}},
	{aisle: "Snacks & Packaged", keywords: []string{"biscuit", "chips", "namkeen", "chocolate", "jam", "honey", "pickle", "achar", "papad", "cornflakes", "muesli", "peanut butter", "maggi", "instant"}},
	{aisle: "Household", keywords: []string{"detergent", "soap", "shampoo", "toothpaste", "tissue", "dishwash", "vim", "harpic", "phenyl", "garbage bag", "foil", "cling", "sponge", "scrub"}},
	{aisle: "Vegetables", keywords: []string{"onion", "pyaz", "tomato", "tamatar", "potato", "aloo", "garlic", "lahsun", "ginger", "adrak", "chilli", "chili", "mirchi", "capsicum", "bell pepper", "carrot", "gajar", "peas", "matar", "beans", "cauliflower", "gobi", "cabbage", "spinach", "palak", "methi", "fenugreek", "coriander", "dhania", "cilantro", "mint", "pudina", "curry leaf", "curry leaves", "kadi patta", "brinjal", "baingan", "eggplant", "okra", "bhindi", "lady finger", "cucumber", "kheera", "bottle gourd", "lauki", "ridge gourd", "turai", "bitter gourd", "karela", "pumpkin", "kaddu", "radish", "mooli", "beetroot", "sweet potato", "shakarkandi", "drumstick", "moringa", "mushroom", "corn", "lettuce", "spring onion", "lemon", "lime", "nimbu", "zucchini", "broccoli", "raw banana", "plantain", "yam", "colocasia", "arbi", "jackfruit", "kathal", "tinda", "parwal", "pointed gourd", "cluster beans", "gawar", "celery", "leek", "avocado"}},
}

func init() {
	// Whole-word matches (plurals allowed), so "salt" doesn't match "unsalted butter".
	for i := range aisleRules {
		quoted := make([]string, len(aisleRules[i].keywords))
		for j, k := range aisleRules[i].keywords {
			quoted[j] = regexp.QuoteMeta(k)
		}
		aisleRules[i].re = regexp.MustCompile(`\b(?:` + strings.Join(quoted, "|") + `)(?:s|es)?\b`)
	}
}

func classifyAisle(name string) string {
	n := strings.ToLower(name)
	for _, r := range aisleRules {
		if r.re.MatchString(n) {
			return r.aisle
		}
	}
	return "Other"
}

func isAisle(a string) bool {
	for _, x := range aisles {
		if x == a {
			return true
		}
	}
	return false
}

func formatFloat(f float64) string { return strconv.FormatFloat(round2(f), 'f', -1, 64) }
