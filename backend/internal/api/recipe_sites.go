package api

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Some sites publish schema.org Recipe data with the ingredients and steps left empty,
// so their pages are read directly. Each parser fills in what it can find.
var siteParsers = map[string]func(doc *html.Node, out *importedRecipe){
	"bongeats.com": parseBongEats,
}

// applySiteParser refines a parsed recipe with the site's own page layout, if known.
func applySiteParser(page string, u *url.URL, out *importedRecipe) bool {
	if u == nil {
		return false
	}
	parse := siteParsers[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")]
	if parse == nil {
		return false
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return false
	}
	parse(doc, out)
	return true
}

// parseBongEats reads bongeats.com's recipe layout: ingredient groups as <h3> + <ul>
// inside .recipe-ingredients (amount in <strong>), and numbered steps in .recipe-process.
func parseBongEats(doc *html.Node, out *importedRecipe) {
	if box := findByClass(doc, "recipe-ingredients"); box != nil {
		var ings []RecipeIngredient
		var notes []string
		section := ""
		multi := countChildren(box, "h3") > 1
		for c := box.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "h3", "h4":
				section = nodeText(c)
			case "ul", "ol":
				for li := c.FirstChild; li != nil; li = li.NextSibling {
					if li.Type != html.ElementNode || li.Data != "li" {
						continue
					}
					line := nodeText(li)
					if line == "" {
						continue
					}
					p := parseIngredientLine(line)
					// Groups like "Marinate the fish" say which step an amount is for.
					var parts []string
					if multi && section != "" {
						parts = append(parts, section)
					}
					if p.Note != "" {
						parts = append(parts, p.Note)
					}
					note := strings.Join(parts, "; ")
					ings = append(ings, RecipeIngredient{Name: p.Name, Quantity: p.Quantity, Unit: p.Unit, Note: note, Aisle: classifyAisle(p.Name)})
				}
			case "p":
				if t := nodeText(c); t != "" {
					notes = append(notes, t)
				}
			}
		}
		if len(ings) > 0 {
			out.Ingredients = ings
		}
		if len(notes) > 0 {
			out.Description = truncate(strings.TrimSpace(out.Description+"\n\n"+strings.Join(notes, "\n")), 1200)
		}
	}
	if box := findByClass(doc, "recipe-process"); box != nil {
		var steps []string
		heading := ""
		var walk func(n *html.Node)
		walk = func(n *html.Node) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode {
					continue
				}
				switch c.Data {
				case "h3", "h4":
					heading = nodeText(c)
				case "li":
					if t := nodeText(c); t != "" {
						if heading != "" {
							t = heading + ": " + t
							heading = ""
						}
						steps = append(steps, t)
					}
				case "p":
					if t := nodeText(c); t != "" {
						steps = append(steps, t)
					}
				default:
					walk(c)
				}
			}
		}
		walk(box)
		if len(steps) > 0 {
			out.Steps = steps
		}
	}
	if out.Servings == 2 { // the default: look for "Serves N" on the page
		if y := findByClass(doc, "recipe-yield"); y != nil {
			if n := firstInt(nodeText(y)); n > 0 && n <= 50 {
				out.Servings = n
			}
		}
	}
	out.Diet = guessDiet(out.Ingredients, "")
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}

func findByClass(n *html.Node, class string) *html.Node {
	if n.Type == html.ElementNode && hasClass(n, class) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findByClass(c, class); f != nil {
			return f
		}
	}
	return nil
}

func countChildren(n *html.Node, tag string) int {
	k := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			k++
		}
	}
	return k
}

// Zero-width characters some editors leave in ("bay leaves\u200d").
var invisibleChars = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "", "\ufeff", "", "\u00a0", " ")

// nodeText is the element's visible text with whitespace collapsed.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
			if n.Data == "br" {
				b.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && (n.Data == "strong" || n.Data == "b") {
			b.WriteByte(' ') // "<strong>10 g</strong>lime juice"
		}
	}
	walk(n)
	return strings.Join(strings.Fields(invisibleChars.Replace(b.String())), " ")
}
