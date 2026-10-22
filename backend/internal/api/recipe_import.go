package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// safeHTTPClient refuses to connect to private, loopback and link-local addresses, so a
// pasted URL can't be used to probe the home network or the server itself. The check runs
// on the resolved IP at connect time, which also covers redirects and DNS rebinding.
var safeHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 8 * time.Second,
			Control: func(network, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				ip := net.ParseIP(host)
				if ip == nil || !publicIP(ip) {
					return errors.New("destination not allowed")
				}
				return nil
			},
		}).DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          4,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("unsupported redirect")
		}
		return nil
	},
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func publicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() || cgnat.Contains(ip))
}

func fetch(ctx context.Context, rawURL string, max int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 HomeOps")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-IN,en;q=0.9")
	resp, err := safeHTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("site answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max))
	return body, resp.Request.URL.String(), err
}

var (
	ldJSONRe  = regexp.MustCompile(`(?is)<script[^>]+type=["']?application/ld\+json["']?[^>]*>(.*?)</script>`)
	ogRe      = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:(title|image|description)["'][^>]+content=["']([^"']*)["']`)
	ogRevRe   = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']*)["'][^>]+property=["']og:(title|image|description)["']`)
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
	durRe     = regexp.MustCompile(`(?i)^P(?:(\d+)D)?T?(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)
	digitsRe  = regexp.MustCompile(`\d+`)
)

type importedRecipe struct {
	Recipe
	Warnings []string `json:"warnings"`
}

// importRecipe fetches a recipe page and reads its schema.org Recipe data (which nearly
// every recipe site publishes for Google). The result is a draft for the user to review.
func (s *Server) importRecipe(w http.ResponseWriter, r *http.Request) error {
	var in struct{ URL string }
	if err := decode(r, &in); err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errBadRequest("Paste a full link starting with https://")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	page, finalURL, err := fetch(ctx, u.String(), 5<<20)
	if err != nil {
		return errBadRequest("Couldn't open that page: " + err.Error())
	}
	out := parseRecipePage(string(page))
	if out.Title == "" {
		return errBadRequest("No recipe found on that page")
	}
	out.SourceURL = finalURL
	if img := out.Photo; img != "" {
		out.Photo = ""
		if imgURL, err := u.Parse(img); err == nil {
			if data, _, err := fetch(ctx, imgURL.String(), maxUploadBytes); err == nil {
				if id, err := s.storeImage(ctx, bytes.NewReader(data)); err == nil {
					out.Photo = id
				}
			}
		}
		if out.Photo == "" {
			out.Warnings = append(out.Warnings, "The photo couldn't be downloaded.")
		}
	}
	if len(out.Ingredients) == 0 {
		out.Warnings = append(out.Warnings, "No ingredient list was found; add them by hand.")
	}
	return ok(w, out)
}

func parseRecipePage(page string) importedRecipe {
	var out importedRecipe
	out.Diet = "veg"
	out.Servings = 2
	out.Category = "main"
	out.MealTypes, out.Tags, out.Steps = []string{}, []string{}, []string{}
	out.Ingredients = []RecipeIngredient{}

	var rec map[string]any
	for _, m := range ldJSONRe.FindAllStringSubmatch(page, -1) {
		var v any
		if json.Unmarshal([]byte(strings.TrimSpace(html.UnescapeString(m[1]))), &v) != nil {
			// Some sites HTML-escape only parts of the JSON; try raw.
			if json.Unmarshal([]byte(strings.TrimSpace(m[1])), &v) != nil {
				continue
			}
		}
		if rec = findRecipe(v); rec != nil {
			break
		}
	}
	if rec == nil {
		// Fall back to Open Graph so at least title/photo/description come through.
		og := map[string]string{}
		for _, m := range ogRe.FindAllStringSubmatch(page, -1) {
			og[strings.ToLower(m[1])] = html.UnescapeString(m[2])
		}
		for _, m := range ogRevRe.FindAllStringSubmatch(page, -1) {
			og[strings.ToLower(m[2])] = html.UnescapeString(m[1])
		}
		out.Title = og["title"]
		if out.Title == "" {
			if m := titleRe.FindStringSubmatch(page); m != nil {
				out.Title = strings.TrimSpace(html.UnescapeString(m[1]))
			}
		}
		out.Description = og["description"]
		out.Photo = og["image"]
		if out.Title != "" {
			out.Warnings = append(out.Warnings, "This page has no structured recipe data; only the title and photo were imported.")
		}
		return out
	}

	out.Title = cleanText(str(rec["name"]))
	out.Description = truncate(cleanText(str(rec["description"])), 600)
	out.Photo = imageURL(rec["image"])
	if n := firstInt(rec["recipeYield"]); n > 0 && n <= 50 {
		out.Servings = n
	}
	if m := durationMinutes(str(rec["prepTime"])); m > 0 {
		out.PrepMinutes = &m
	}
	if m := durationMinutes(str(rec["cookTime"])); m > 0 {
		out.CookMinutes = &m
	} else if t := durationMinutes(str(rec["totalTime"])); t > 0 {
		out.CookMinutes = &t
	}
	if c := strs(rec["recipeCuisine"]); len(c) > 0 {
		out.Cuisine = cleanText(c[0])
	}
	for _, line := range strs(rec["recipeIngredient"]) {
		line = cleanText(line)
		if line == "" {
			continue
		}
		p := parseIngredientLine(line)
		out.Ingredients = append(out.Ingredients, RecipeIngredient{Name: p.Name, Quantity: p.Quantity, Unit: p.Unit, Note: p.Note, Aisle: classifyAisle(p.Name)})
	}
	out.Steps = instructions(rec["recipeInstructions"])
	for _, k := range strings.Split(strings.Join(strs(rec["keywords"]), ","), ",") {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" && len(out.Tags) < 8 && len(k) < 30 {
			out.Tags = append(out.Tags, k)
		}
	}
	cats := strings.ToLower(strings.Join(strs(rec["recipeCategory"]), " ") + " " + out.Title)
	out.MealTypes, out.Category = guessMeal(cats)
	out.Diet = guessDiet(out.Ingredients, strings.ToLower(strings.Join(strs(rec["suitableForDiet"]), " ")))
	return out
}

// findRecipe walks JSON-LD (arrays, @graph, mainEntity) for the first Recipe object.
func findRecipe(v any) map[string]any {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if r := findRecipe(x); r != nil {
				return r
			}
		}
	case map[string]any:
		for _, typ := range strs(t["@type"]) {
			if strings.EqualFold(typ, "Recipe") {
				return t
			}
		}
		for _, k := range []string{"@graph", "mainEntity", "mainEntityOfPage", "itemListElement"} {
			if r := findRecipe(t[k]); r != nil {
				return r
			}
		}
	}
	return nil
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		if len(t) > 0 {
			return str(t[0])
		}
	case map[string]any:
		if s, ok := t["text"].(string); ok {
			return s
		}
		if s, ok := t["name"].(string); ok {
			return s
		}
	}
	return ""
}

func strs(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := []string{}
		for _, x := range t {
			if s := str(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case nil:
		return nil
	}
	if s := str(v); s != "" {
		return []string{s}
	}
	return nil
}

func imageURL(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			return imageURL(t[len(t)-1]) // sites list sizes smallest → largest
		}
	case map[string]any:
		if s, ok := t["url"].(string); ok {
			return s
		}
	}
	return ""
}

func instructions(v any) []string {
	out := []string{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			for _, line := range strings.Split(cleanTextKeepLines(t), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					out = append(out, line)
				}
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		case map[string]any:
			if items, ok := t["itemListElement"]; ok { // HowToSection
				walk(items)
				return
			}
			if s := cleanText(str(t)); s != "" {
				out = append(out, s)
			}
		}
	}
	walk(v)
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(html.UnescapeString(s), " "))), " ")
}

func cleanTextKeepLines(s string) string {
	s = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`).ReplaceAllString(html.UnescapeString(s), "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = cleanText(l)
	}
	return strings.Join(lines, "\n")
}

func firstInt(v any) int {
	for _, s := range strs(v) {
		if m := digitsRe.FindString(s); m != "" {
			n, _ := strconv.Atoi(m)
			return n
		}
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func durationMinutes(s string) int {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	return n(1)*24*60 + n(2)*60 + n(3) + (n(4)+59)/60
}

func guessMeal(text string) ([]string, string) {
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("breakfast", "poha", "upma", "idli", "dosa", "paratha", "uttapam", "cheela", "omelette", "pancake"):
		return []string{"breakfast"}, "breakfast"
	case has("snack", "starter", "appetizer", "pakora", "pakoda", "samosa", "chaat", "tikki", "vada", "bhaji", "cutlet"):
		return []string{"snacks"}, "snack"
	case has("dessert", "sweet", "halwa", "kheer", "ladoo", "laddu", "barfi", "payasam", "cake", "pudding"):
		return []string{"lunch", "dinner"}, "dessert"
	case has("drink", "beverage", "lassi", "chai", "tea", "coffee", "smoothie", "juice", "sherbet"):
		return []string{"breakfast", "snacks"}, "drink"
	case has("dal", "daal", "sambar", "rasam"):
		return []string{"lunch", "dinner"}, "dal"
	case has("roti", "chapati", "naan", "puri", "bhatura", "kulcha", "thepla", "bread"):
		return []string{"lunch", "dinner"}, "bread"
	case has("biryani", "pulao", "pulav", "fried rice", "khichdi", "rice"):
		return []string{"lunch", "dinner"}, "rice"
	case has("sabzi", "sabji", "subzi", "bhaji", "poriyal", "thoran", "fry"):
		return []string{"lunch", "dinner"}, "sabzi"
	case has("curry", "masala", "korma", "gravy", "kadai", "makhani", "kofta"):
		return []string{"lunch", "dinner"}, "curry"
	case has("soup"):
		return []string{"dinner"}, "soup"
	case has("salad", "raita"):
		return []string{"lunch", "dinner"}, "salad"
	case has("chutney", "pickle"):
		return []string{"breakfast", "lunch", "dinner"}, "chutney"
	}
	return []string{"lunch", "dinner"}, "main"
}

func guessDiet(ings []RecipeIngredient, declared string) string {
	if strings.Contains(declared, "vegan") {
		return "vegan"
	}
	egg := false
	for _, i := range ings {
		switch classifyAisle(i.Name) {
		case "Meat & Fish":
			return "nonveg"
		}
		if regexp.MustCompile(`(?i)\beggs?\b`).MatchString(i.Name) {
			egg = true
		}
	}
	if egg {
		return "egg"
	}
	return "veg"
}
