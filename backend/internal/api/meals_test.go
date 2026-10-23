package api

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMealsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	s, ts, _ := startTestServer(t, 54330)
	c := newClient(t, ts.URL)
	c.do("POST", "/api/setup", map[string]any{"householdName": "Home", "email": "cook@example.com", "username": "cook", "displayName": "Cook", "password": "hunter2hunter2"}, 204, nil)

	// Photo upload is re-encoded with a thumbnail.
	photo := uploadTestImage(t, c)

	// Ingredient parsing helper.
	var parsed []map[string]any
	c.do("POST", "/api/ingredients/parse", map[string]any{"lines": []string{"1 cup toor dal", "", "2 tomatoes, chopped"}}, 200, &parsed)
	if len(parsed) != 2 || parsed[0]["aisle"] != "Pulses & Lentils" {
		t.Fatalf("parse: %v", parsed)
	}

	q := func(v float64) *float64 { return &v }
	var dal, rice, rajma Recipe
	c.do("POST", "/api/recipes", Recipe{Title: "Dal tadka", Category: "dal", Diet: "veg", Servings: 2, MealTypes: []string{"lunch", "dinner"}, Photo: photo,
		Steps:       []string{"Pressure cook dal", " ", "Temper"},
		Ingredients: []RecipeIngredient{{Name: "toor dal", Quantity: q(1), Unit: "cup"}, {Name: "Ghee", Quantity: q(1), Unit: "tbsp"}, {Name: "Salt"}}}, 200, &dal)
	if dal.ID == "" || len(dal.Ingredients) != 3 || len(dal.Steps) != 2 || dal.Ingredients[0].Name != "Toor dal" {
		t.Fatalf("dal: %+v", dal)
	}
	c.do("POST", "/api/recipes", Recipe{Title: "Jeera rice", Category: "rice", Diet: "veg", Servings: 2, MealTypes: []string{"lunch", "dinner"},
		Ingredients: []RecipeIngredient{{Name: "Basmati rice", Quantity: q(200), Unit: "g"}, {Name: "ghee", Quantity: q(2), Unit: "tsp"}}}, 200, &rice)
	if rice.Ingredients[1].IngredientID != dal.Ingredients[1].IngredientID {
		t.Fatal("ingredient catalog should dedupe case-insensitively")
	}
	c.do("POST", "/api/recipes", Recipe{Title: "Rajma", Category: "curry", Diet: "veg", Servings: 4, MealTypes: []string{"lunch"},
		PrepNote: "Soak rajma overnight", PrepLeadHours: 10,
		Ingredients: []RecipeIngredient{{Name: "Rajma", Quantity: q(1), Unit: "cup"}}}, 200, &rajma)
	c.do("POST", "/api/recipes", Recipe{Title: "", Diet: "veg"}, 400, nil)

	// Pantry: 1 kg rice and some ghee.
	var ricePantry, gheePantry PantryItem
	c.do("POST", "/api/pantry", map[string]any{"name": "basmati rice", "quantity": 1, "unit": "kg", "lowThreshold": 0.5}, 200, &ricePantry)
	c.do("POST", "/api/pantry", map[string]any{"name": "Ghee", "quantity": 200, "unit": "ml"}, 200, &gheePantry)

	// Plan: dal + rice for lunch today and tomorrow (4 servings tomorrow).
	h, _ := s.getHousehold(t.Context(), dal.householdOf(t, s))
	today := localToday(h, time.Now())
	d0, d1 := today.Format(time.DateOnly), today.AddDate(0, 0, 1).Format(time.DateOnly)
	var m1, m2 MealItem
	c.do("POST", "/api/meals", map[string]any{"id": "11111111-1111-4111-8111-111111111111", "date": d0, "slot": "lunch", "recipeId": dal.ID}, 201, &m1)
	c.do("POST", "/api/meals", map[string]any{"id": "11111111-1111-4111-8111-111111111111", "date": d0, "slot": "lunch", "recipeId": dal.ID}, 200, nil) // idempotent
	c.do("POST", "/api/meals", map[string]any{"date": d0, "slot": "lunch", "recipeId": rice.ID}, 201, &m2)
	c.do("POST", "/api/meals", map[string]any{"date": d1, "slot": "lunch", "recipeId": dal.ID, "servings": 4}, 201, nil)
	c.do("POST", "/api/meals", map[string]any{"date": d1, "slot": "dinner", "title": "Leftovers"}, 201, nil)
	c.do("POST", "/api/meals", map[string]any{"date": d1, "slot": "brunch", "title": "x"}, 400, nil)
	if m1.Title != "Dal tadka" || m1.Servings != 2 {
		t.Fatalf("meal: %+v", m1)
	}

	// Grocery generation: dal 1 cup + 2 cups (4 servings) = 3 cups; rice covered by pantry;
	// ghee 1 tbsp + 2 tbsp + 2 tsp = 55 ml, covered by 200 ml; salt has no pantry entry → listed.
	var gen map[string]int
	c.do("POST", "/api/grocery/generate", map[string]any{"from": d0, "to": d1}, 200, &gen)
	var list []GroceryItem
	c.do("GET", "/api/grocery", nil, 200, &list)
	names := map[string]GroceryItem{}
	for _, it := range list {
		names[it.Name] = it
	}
	if dalItem, ok := names["Toor dal"]; !ok || *dalItem.Quantity != 3 || dalItem.Unit != "cup" || dalItem.Aisle != "Pulses & Lentils" {
		t.Fatalf("toor dal on list: %+v (list %v)", dalItem, names)
	}
	if _, ok := names["Basmati rice"]; ok {
		t.Fatal("rice should be covered by the pantry")
	}
	if _, ok := names["Ghee"]; ok {
		t.Fatal("ghee should be covered by the pantry")
	}
	if _, ok := names["Salt"]; !ok || gen["covered"] != 2 {
		t.Fatalf("salt should be listed; gen=%v", gen)
	}
	if !notShopped("Hot water") || notShopped("Watermelon") {
		t.Fatal("water filter")
	}
	if roundUpForShopping(0.92, "cup") != 1 || roundUpForShopping(3.2, "") != 4 || roundUpForShopping(230, "g") != 250 || roundUpForShopping(1.5, "kg") != 1.5 {
		t.Fatal("shopping rounding")
	}
	// Regenerating replaces, not duplicates.
	c.do("POST", "/api/grocery/generate", map[string]any{"from": d0, "to": d1}, 200, nil)
	var again []GroceryItem
	c.do("GET", "/api/grocery", nil, 200, &again)
	if len(again) != len(list) {
		t.Fatalf("regenerate duplicated items: %d vs %d", len(again), len(list))
	}
	for _, it := range again {
		names[it.Name] = it
	}

	// Manual add with free text.
	var onions GroceryItem
	c.do("POST", "/api/grocery", map[string]any{"text": "2 kg onions"}, 201, &onions)
	if onions.Name != "Onions" || *onions.Quantity != 2 || onions.Unit != "kg" || onions.Aisle != "Vegetables" {
		t.Fatalf("onions: %+v", onions)
	}

	// Shop: tick dal + onions, finish → into pantry, trip recorded.
	c.do("PATCH", "/api/grocery/"+names["Toor dal"].ID, map[string]any{"checked": true}, 200, nil)
	c.do("PATCH", "/api/grocery/"+onions.ID, map[string]any{"checked": true}, 200, nil)
	var done map[string]any
	c.do("POST", "/api/grocery/complete", map[string]any{"total": 640, "store": "Big Bazaar"}, 200, &done)
	c.do("POST", "/api/grocery/complete", map[string]any{}, 400, nil)
	var pantry []PantryItem
	c.do("GET", "/api/pantry", nil, 200, &pantry)
	byName := map[string]PantryItem{}
	for _, p := range pantry {
		byName[p.Name] = p
	}
	if p := byName["Toor dal"]; p.Quantity == nil || *p.Quantity != 3 || p.Unit != "cup" {
		t.Fatalf("dal in pantry: %+v", p)
	}
	if byName["Onions"].Location != "fridge" {
		t.Fatalf("vegetables go to the fridge: %+v", byName["Onions"])
	}
	var trips []GroceryTrip
	c.do("GET", "/api/grocery/trips", nil, 200, &trips)
	if len(trips) != 1 || *trips[0].Total != 640 || trips[0].ItemCount != 2 {
		t.Fatalf("trips: %+v", trips)
	}

	// Cooking today's rice deducts 200 g from 1 kg; un-cooking restores it.
	c.do("PATCH", "/api/meals/"+m2.ID, map[string]any{"cooked": true}, 200, nil)
	c.do("GET", "/api/pantry", nil, 200, &pantry)
	if q := pantryQty(pantry, "Basmati rice"); q != 0.8 {
		t.Fatalf("rice after cooking: %v", q)
	}
	if q := pantryQty(pantry, "Ghee"); q != 190 {
		t.Fatalf("ghee after cooking: %v (2 tsp = 10 ml)", q)
	}
	c.do("PATCH", "/api/meals/"+m2.ID, map[string]any{"cooked": false}, 200, nil)
	c.do("GET", "/api/pantry", nil, 200, &pantry)
	if q := pantryQty(pantry, "Basmati rice"); q != 1 {
		t.Fatalf("rice after undo: %v", q)
	}

	// Restock: rice low threshold 0.5 kg; set to 0.4 → restock adds it.
	c.do("PATCH", "/api/pantry/"+ricePantry.ID, map[string]any{"quantity": 0.4}, 200, nil)
	var restock map[string]int
	c.do("POST", "/api/pantry/restock", nil, 200, &restock)
	if restock["added"] != 1 {
		t.Fatalf("restock: %v", restock)
	}
	c.do("POST", "/api/pantry/restock", nil, 200, &restock)
	if restock["added"] != 0 {
		t.Fatal("restock should not duplicate")
	}

	// Suggestions: with dal already at lunch, rice/rajma rank; dal excluded from its own slot.
	var sugg []Suggestion
	c.do("GET", "/api/meals/suggest?date="+d0+"&slot=lunch", nil, 200, &sugg)
	for _, sg := range sugg {
		if sg.RecipeID == dal.ID || sg.RecipeID == rice.ID {
			t.Fatalf("already-planned dish suggested: %+v", sg)
		}
	}
	if len(sugg) == 0 || sugg[0].RecipeID != rajma.ID {
		t.Fatalf("expected rajma first: %+v", sugg)
	}
	var fill map[string]int
	c.do("POST", "/api/meals/autofill", map[string]any{"from": d0, "to": today.AddDate(0, 0, 2).Format(time.DateOnly), "slots": []string{"dinner"}}, 200, &fill)
	// Dinners on d0 and d2 get a plate (curry/dal + rice/bread); d1 dinner already has leftovers.
	if fill["created"] < 2 || fill["created"] > 4 {
		t.Fatalf("autofill: %v", fill)
	}

	// Renaming a recipe updates planned titles; deleting keeps history as text.
	dal.Title = "Dal fry"
	c.do("PUT", "/api/recipes/"+dal.ID, dal, 200, nil)
	var meals []MealItem
	c.do("GET", "/api/meals?from="+d0+"&to="+d1, nil, 200, &meals)
	if meals[0].Title != "Dal fry" {
		t.Fatalf("rename not propagated: %+v", meals[0])
	}
	c.do("DELETE", "/api/recipes/"+dal.ID, nil, 204, nil)
	c.do("GET", "/api/meals?from="+d0+"&to="+d1, nil, 200, &meals)
	if meals[0].RecipeID != nil || meals[0].Title != "Dal fry" {
		t.Fatalf("history after delete: %+v", meals[0])
	}

	// Prep reminder fires 10h before tomorrow's rajma lunch.
	c.do("POST", "/api/meals", map[string]any{"date": d1, "slot": "lunch", "recipeId": rajma.ID}, 201, nil)
	loc := h.Location()
	lunch := time.Date(today.Year(), today.Month(), today.Day()+1, 13, 30, 0, 0, loc)
	if err := s.prepReminders(t.Context(), h, lunch.Add(-9*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var inbox struct{ Items []inboxItem }
	c.do("GET", "/api/notifications", nil, 200, &inbox)
	found := false
	for _, n := range inbox.Items {
		found = found || (strings.Contains(n.Title, "Prep for Rajma") && n.Body == "Soak rajma overnight")
	}
	if !found {
		t.Fatalf("prep reminder missing: %+v", inbox.Items)
	}

	// Photo is served (auth required).
	resp, err := c.http.Get(ts.URL + "/api/files/" + photo + "?thumb=1")
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumb: %v %v", err, resp)
	}
	resp, _ = http.Get(ts.URL + "/api/files/" + photo)
	if resp.StatusCode != 401 {
		t.Fatalf("files must require auth, got %d", resp.StatusCode)
	}
}

func pantryQty(items []PantryItem, name string) float64 {
	for _, p := range items {
		if p.Name == name && p.Quantity != nil {
			return round2(*p.Quantity)
		}
	}
	return -1
}

func (r Recipe) householdOf(t *testing.T, s *Server) string {
	var hid string
	if err := s.db.QueryRow(t.Context(), `SELECT household_id FROM recipes WHERE id = $1`, r.ID).Scan(&hid); err != nil {
		t.Fatal(err)
	}
	return hid
}

func uploadTestImage(t *testing.T, c *testClient) string {
	img := image.NewRGBA(image.Rect(0, 0, 2400, 1200))
	for x := 0; x < 2400; x += 7 {
		img.Set(x, x/2, color.RGBA{200, 80, 20, 255})
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "photo.png")
	io.Copy(fw, &buf)
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/files", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "homeops")
	resp, err := c.http.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %v", err, resp)
	}
	var out struct{ ID string }
	jsonDecode(t, resp.Body, &out)
	return out.ID
}

func TestRecipeImportParsing(t *testing.T) {
	page := `<html><head><title>x</title>
	<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebPage"},
	{"@type":["Recipe"],"name":"Paneer Butter Masala","description":"Rich &amp; creamy","image":["https://e.x/s.jpg","https://e.x/l.jpg"],
	"recipeYield":["4","4 servings"],"prepTime":"PT15M","cookTime":"PT1H5M","recipeCuisine":"North Indian","recipeCategory":"Main Course",
	"keywords":"paneer, curry","recipeIngredient":["250 g paneer, cubed","2 tbsp butter","1 cup tomato puree","Salt to taste"],
	"recipeInstructions":[{"@type":"HowToSection","name":"Gravy","itemListElement":[{"@type":"HowToStep","text":"Heat <b>butter</b>."},{"@type":"HowToStep","text":"Add puree."}]},
	{"@type":"HowToStep","text":"Add paneer."}]}]}</script></head></html>`
	r := parseRecipePage(page, nil)
	if r.Title != "Paneer Butter Masala" || r.Description != "Rich & creamy" || r.Photo != "https://e.x/l.jpg" || r.Servings != 4 {
		t.Fatalf("basics: %+v", r)
	}
	if *r.PrepMinutes != 15 || *r.CookMinutes != 65 || r.Cuisine != "North Indian" || r.Diet != "veg" || r.Category != "curry" {
		t.Fatalf("meta: prep=%v cook=%v %+v", *r.PrepMinutes, *r.CookMinutes, r)
	}
	if len(r.Ingredients) != 4 || r.Ingredients[0].Name != "Paneer" || *r.Ingredients[0].Quantity != 250 || r.Ingredients[0].Unit != "g" {
		t.Fatalf("ingredients: %+v", r.Ingredients)
	}
	if len(r.Steps) != 3 || r.Steps[0] != "Heat butter ." && r.Steps[0] != "Heat butter." {
		t.Fatalf("steps: %q", r.Steps)
	}
	if len(r.Tags) != 2 {
		t.Fatalf("tags: %v", r.Tags)
	}
	nonveg := parseRecipePage(`<script type="application/ld+json">{"@type":"Recipe","name":"Chicken curry","recipeIngredient":["500 g chicken"]}</script>`, nil)
	if nonveg.Diet != "nonveg" {
		t.Fatalf("diet: %s", nonveg.Diet)
	}
	og := parseRecipePage(`<meta property="og:title" content="Aloo Paratha"><meta property="og:image" content="https://e.x/a.jpg">`, nil)
	if og.Title != "Aloo Paratha" || og.Photo != "https://e.x/a.jpg" || len(og.Warnings) == 0 {
		t.Fatalf("og fallback: %+v", og)
	}
}

func TestPublicIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"8.8.8.8": true, "142.250.183.14": true, "127.0.0.1": false, "10.0.0.5": false, "192.168.1.10": false,
		"172.16.3.4": false, "169.254.169.254": false, "100.64.1.1": false, "::1": false, "fd00::1": false, "0.0.0.0": false,
	} {
		if got := publicIP(net.ParseIP(ip)); got != want {
			t.Errorf("%s: got %v", ip, got)
		}
	}
}

// bongeats.com's JSON-LD leaves ingredients and steps empty; they're read from the page.
func TestBongEatsImport(t *testing.T) {
	page := `<script type="application/ld+json">{"@context":"https://schema.org/","@type":"Recipe","name":"Test Machher Jhol",
		"description":"A light fish curry","image":["/x/a_1x1.jpg","/x/a_4x3.jpg","/x/a_16x9.jpg"],"prepTime":"PT10M","cookTime":"PT30M",
		"recipeYield":"4","recipeCategory":"fish","recipeIngredient":[],"recipeInstructions":[]}</script>
	<div class="recipe-info recipe-yield"><div>Serves</div><div>4</div></div>
	<div class="rich-text-block recipe-ingredients shadow-1 w-richtext">
		<h3>Marinate the fish</h3>
		<ul role="list"><li><strong>500 g</strong> rui machh</li><li><strong>1 tsp </strong>salt</li></ul>
		<h3>For the jhol</h3>
		<ul role="list"><li><strong>½ tsp</strong> kaalo jeere (nigella seeds)</li><li><strong>100 g</strong> peyaj (sliced)</li>
		<li><strong>200 g</strong> fragrant, small-grained, gobindobhog rice</li><li><strong>4 pcs</strong> bay leaves<strong>&#8205;</strong></li><li>water</li></ul>
	</div>
	<div class="rich-text-block recipe-process w-richtext"><ol role="list"><li>Smear the fish with salt.</li><li>Fry, then add <a href="/spices/x">the spices</a>.</li></ol></div>`
	u, _ := url.Parse("https://www.bongeats.com/recipe/test-machher-jhol")
	r := parseRecipePage(page, u)
	if r.Title != "Test Machher Jhol" || r.Servings != 4 || r.Diet != "nonveg" || r.Category != "curry" || r.Photo != "/x/a_4x3.jpg" {
		t.Fatalf("header: %+v", r.Recipe)
	}
	if len(r.Steps) != 2 || r.Steps[1] != "Fry, then add the spices." {
		t.Fatalf("steps: %q", r.Steps)
	}
	want := []struct {
		name, unit, note, aisle string
		qty                     float64
	}{
		{"Rui machh", "g", "Marinate the fish", "Meat & Fish", 500},
		{"Salt", "tsp", "Marinate the fish", "Spices & Masala", 1},
		{"Kaalo jeere", "tsp", "For the jhol; nigella seeds", "Spices & Masala", 0.5},
		{"Peyaj", "g", "For the jhol; sliced", "Vegetables", 100},
		{"Gobindobhog rice", "g", "For the jhol; fragrant, small-grained", "Grains & Flour", 200},
		{"Bay leaves", "pcs", "For the jhol", "Spices & Masala", 4},
	}
	if len(r.Ingredients) != len(want)+1 {
		t.Fatalf("got %d ingredients: %+v", len(r.Ingredients), r.Ingredients)
	}
	for i, w := range want {
		g := r.Ingredients[i]
		if g.Name != w.name || g.Unit != w.unit || g.Note != w.note || g.Aisle != w.aisle || g.Quantity == nil || *g.Quantity != w.qty {
			t.Errorf("ingredient %d: got %+v (qty %v), want %+v", i, g, g.Quantity, w)
		}
	}
	if w := r.Ingredients[len(want)]; w.Name != "Water" || w.Quantity != nil {
		t.Errorf("water: %+v", w)
	}
}
