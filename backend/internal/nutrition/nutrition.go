// Package nutrition estimates what a recipe provides from its ingredient list, and what
// a person needs per day.
//
// Food values are per 100 g of the edible, raw food, taken from the Indian Food
// Composition Tables (IFCT 2017, NIN) where available and USDA FoodData Central
// otherwise. Daily needs follow the ICMR-NIN 2020 Recommended Dietary Allowances.
// Home cooking varies a lot (oil especially), so results are estimates, good for
// spotting patterns and gaps rather than counting single calories.
package nutrition

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Nutrients is an amount of each tracked nutrient. Units: kcal; g for protein, carbs,
// fat and fibre; mg for minerals and vitamin C; µg for vitamin A (RAE), folate and B12.
type Nutrients struct {
	Kcal      float64 `json:"kcal"`
	Protein   float64 `json:"protein"`
	Carbs     float64 `json:"carbs"`
	Fat       float64 `json:"fat"`
	Fibre     float64 `json:"fibre"`
	Iron      float64 `json:"iron"`
	Calcium   float64 `json:"calcium"`
	VitC      float64 `json:"vitC"`
	VitA      float64 `json:"vitA"`
	Folate    float64 `json:"folate"`
	B12       float64 `json:"b12"`
	Zinc      float64 `json:"zinc"`
	Magnesium float64 `json:"magnesium"`
	Potassium float64 `json:"potassium"`
	Sodium    float64 `json:"sodium"`
}

// Keys lists the nutrient JSON names in display order.
var Keys = []string{"kcal", "protein", "carbs", "fat", "fibre", "iron", "calcium", "vitC", "vitA", "folate", "b12", "zinc", "magnesium", "potassium", "sodium"}

func (n *Nutrients) ptrs() []*float64 {
	return []*float64{&n.Kcal, &n.Protein, &n.Carbs, &n.Fat, &n.Fibre, &n.Iron, &n.Calcium, &n.VitC, &n.VitA, &n.Folate, &n.B12, &n.Zinc, &n.Magnesium, &n.Potassium, &n.Sodium}
}

// Get returns a nutrient by its JSON name.
func (n Nutrients) Get(key string) float64 {
	for i, k := range Keys {
		if k == key {
			return *n.ptrs()[i]
		}
	}
	return 0
}

// Add adds o scaled by f.
func (n *Nutrients) Add(o Nutrients, f float64) {
	dst, src := n.ptrs(), o.ptrs()
	for i := range dst {
		*dst[i] += *src[i] * f
	}
}

// Scale returns n multiplied by f.
func (n Nutrients) Scale(f float64) Nutrients {
	var out Nutrients
	out.Add(n, f)
	return out
}

// Round keeps one decimal (two for B12), so JSON stays readable.
func (n Nutrients) Round() Nutrients {
	out := n
	for i, p := range out.ptrs() {
		d := 10.0
		if Keys[i] == "b12" {
			d = 100
		}
		*p = math.Round(*p*d) / d
	}
	return out
}

// Food is one entry of the composition table.
type Food struct {
	Key     string             `json:"key"`
	Name    string             `json:"name"`
	Group   string             `json:"group,omitempty"` // fish, seafood, chicken, mutton, egg, dairy, fruit… (diet filters)
	Per100  Nutrients          `json:"per100g"`
	Density float64            `json:"-"` // g per ml, for cup/tbsp/tsp measures
	Piece   float64            `json:"-"` // edible g per piece ("2 onions")
	Units   map[string]float64 `json:"-"` // g per unit for this food: garlic clove, ginger inch…
	Edible  float64            `json:"-"` // edible fraction of the bought weight (bone, peel)
	aliases []string
}

var (
	foods   []*Food
	byKey   = map[string]*Food{}
	matcher []aliasRe
)

type aliasRe struct {
	re   *regexp.Regexp
	food *Food
	n    int
}

// Foods returns the composition table, sorted by name.
func Foods() []*Food {
	out := append([]*Food{}, foods...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ByKey looks a food up by key.
func ByKey(key string) *Food { return byKey[key] }

func register(f *Food) {
	if f.Edible == 0 {
		f.Edible = 1
	}
	foods = append(foods, f)
	byKey[f.Key] = f
	for _, a := range append([]string{strings.ToLower(f.Name)}, f.aliases...) {
		a = normalize(a)
		if a == "" {
			continue
		}
		matcher = append(matcher, aliasRe{regexp.MustCompile(`\b` + regexp.QuoteMeta(a) + `(?:s|es)?\b`), f, len(a)})
	}
}

func init() {
	for _, f := range table {
		register(f)
	}
	// Longest alias wins: "mustard oil" before "mustard", "sweet potato" before "potato".
	sort.SliceStable(matcher, func(i, j int) bool { return matcher[i].n > matcher[j].n })
}

var nonLetters = regexp.MustCompile(`[^a-z0-9 ]+`)

func normalize(s string) string {
	s = strings.ToLower(s)
	s = nonLetters.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

var matchCache sync.Map // normalized name → *Food (nil included)

// Match finds the food an ingredient name refers to, or nil.
func Match(name string) *Food {
	n := normalize(name)
	if f, ok := matchCache.Load(n); ok {
		return f.(*Food)
	}
	var found *Food
	for _, a := range matcher {
		if a.re.MatchString(n) {
			found = a.food
			break
		}
	}
	matchCache.Store(n, found)
	return found
}

// Generic weights for count-like units when a food doesn't define its own.
var unitGrams = map[string]float64{"pinch": 0.3, "handful": 30, "sprig": 1, "leaf": 0.4, "slice": 25, "clove": 4, "inch": 6, "pod": 0.3, "stick": 2}

var (
	massUnits   = map[string]float64{"g": 1, "kg": 1000, "mg": 0.001, "oz": 28.35, "lb": 453.6}
	volumeUnits = map[string]float64{"ml": 1, "l": 1000, "tsp": 5, "tbsp": 15, "cup": 240, "fl oz": 29.57}
)

// Grams converts an amount of a food to edible grams. ok is false when the unit can't
// be weighed (a "packet" of something with no known pack size).
func Grams(f *Food, qty float64, unit string) (float64, bool) {
	g, ok := rawGrams(f, qty, unit)
	if !ok {
		return 0, false
	}
	return g * f.Edible, true
}

func rawGrams(f *Food, qty float64, unit string) (float64, bool) {
	if g, ok := massUnits[unit]; ok {
		return qty * g, true
	}
	if ml, ok := volumeUnits[unit]; ok {
		d := f.Density
		if d == 0 {
			d = 1
		}
		return qty * ml * d, true
	}
	if g, ok := f.Units[unit]; ok {
		return qty * g, true
	}
	switch unit {
	case "", "pcs":
		if f.Piece > 0 {
			return qty * f.Piece / f.Edible, true // piece weights are already edible
		}
		return 0, false
	case "dozen":
		if f.Piece > 0 {
			return qty * 12 * f.Piece / f.Edible, true
		}
		return 0, false
	}
	if g, ok := unitGrams[unit]; ok {
		return qty * g, true
	}
	return 0, false
}

// Line is one recipe ingredient.
type Line struct {
	Name     string
	FoodKey  string // chosen food; "" = match by name, "-" = leave out
	Quantity *float64
	Unit     string
}

// Result is a recipe's nutrition.
type Result struct {
	PerServing Nutrients `json:"perServing"`
	// Coverage is the share of measured ingredients that could be weighed and matched.
	Coverage float64  `json:"coverage"`
	Missing  []string `json:"missing"`  // ingredients left out of the estimate
	Contains []string `json:"contains"` // food groups present: fish, egg, chicken…
}

// Resolve returns the food a line refers to, or nil.
func (l Line) Resolve() *Food {
	switch l.FoodKey {
	case "-":
		return nil
	case "":
		return Match(l.Name)
	}
	if f := byKey[l.FoodKey]; f != nil {
		return f
	}
	return Match(l.Name)
}

// Calc totals a recipe and divides by its servings.
func Calc(lines []Line, servings int) Result {
	res := Result{Missing: []string{}, Contains: []string{}}
	var total Nutrients
	measured, matched := 0, 0
	groups := map[string]bool{}
	for _, l := range lines {
		f := l.Resolve()
		if f != nil && f.Group != "" && f.Group != "fruit" && f.Group != "oil" {
			groups[f.Group] = true
		}
		if l.FoodKey == "-" || l.Quantity == nil || *l.Quantity <= 0 {
			continue // "salt to taste": too small to matter, and unknowable
		}
		measured++
		if f == nil {
			res.Missing = append(res.Missing, l.Name)
			continue
		}
		g, ok := Grams(f, *l.Quantity, l.Unit)
		if !ok {
			res.Missing = append(res.Missing, l.Name)
			continue
		}
		matched++
		total.Add(f.Per100, g/100)
	}
	if servings < 1 {
		servings = 1
	}
	res.PerServing = total.Scale(1 / float64(servings)).Round()
	if measured > 0 {
		res.Coverage = math.Round(float64(matched)/float64(measured)*100) / 100
	}
	for g := range groups {
		res.Contains = append(res.Contains, g)
	}
	sort.Strings(res.Contains)
	return res
}
