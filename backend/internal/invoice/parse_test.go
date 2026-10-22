package invoice

import (
	"encoding/json"
	"os"
	"testing"
)

// Fixtures are positioned words extracted from real Zepto and Blinkit tax invoices,
// with customer details removed and product names replaced. Layout, amounts and
// structure are untouched, so totals must still reconcile exactly.
func load(t *testing.T, name string) []Page {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var pages []Page
	if err := json.Unmarshal(b, &pages); err != nil {
		t.Fatal(err)
	}
	return pages
}

func goods(inv *Invoice) []Item {
	var out []Item
	for _, it := range inv.Items {
		if !it.Fee {
			out = append(out, it)
		}
	}
	return out
}

func TestZepto(t *testing.T) {
	inv := Parse(load(t, "zepto.json"))
	if inv.Merchant != "Zepto" || inv.Date != "2026-10-20" || inv.Total != 31000 || len(inv.Warnings) != 0 {
		t.Fatalf("header: %+v", inv)
	}
	g := goods(inv)
	if len(g) != 3 || len(inv.Items) != 3 {
		t.Fatalf("items: %+v", inv.Items)
	}
	want := []struct {
		qty    float64
		amount int64
		pack   float64
		unit   string
	}{{1, 8700, 2.25, "l"}, {2, 7400, 80, "g"}, {1, 14900, 3, "pcs"}}
	for i, w := range want {
		it := g[i]
		if it.Qty != w.qty || it.Amount != w.amount || it.PackQty == nil || *it.PackQty != w.pack || it.PackUnit != w.unit {
			t.Errorf("item %d: %+v (pack %v)", i, it, it.PackQty)
		}
		if it.Name == "" || it.Name == "Item" {
			t.Errorf("item %d has no name", i)
		}
	}
}

func TestBlinkitMergedInvoices(t *testing.T) {
	inv := Parse(load(t, "blinkit.json"))
	if inv.Merchant != "Blinkit" || inv.Date != "2026-10-17" || len(inv.Warnings) != 0 {
		t.Fatalf("header: %+v", inv)
	}
	// One order split into several seller invoices: the total is their sum.
	if inv.Total != 59600 || inv.ItemTotal != 59600 {
		t.Fatalf("total %d, items %d", inv.Total, inv.ItemTotal)
	}
	g := goods(inv)
	if len(g) != 7 {
		t.Fatalf("goods: %d %+v", len(g), g)
	}
	fees := len(inv.Items) - len(g)
	if fees != 9 {
		t.Fatalf("fees: %d", fees)
	}
	var qty2 bool
	for _, it := range g {
		if it.Qty == 2 && it.Amount == 11700 {
			qty2 = true
		}
		if it.Amount == 5700 && (it.PackQty == nil || *it.PackQty != 6 || it.PackUnit != "pcs") {
			t.Errorf("6-pack size: %+v", it)
		}
	}
	if !qty2 {
		t.Error("quantity 2 line not read")
	}
}

func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"Sprite | Lemon-Lime Soft Drink | The Coca-Cola Company 1 pc (2.25 L)": "Sprite Lemon-Lime Soft Drink",
		"Coca-Cola Soft Drink(PET Bottle) (HSN-22021010)":                      "Coca-Cola Soft Drink",
		"Table White Eggs (6 pcs)(Pack)":                                        "Table White Eggs",
		"Aashirvaad Atta 5 kg":                                                  "Aashirvaad Atta",
	}
	for in, want := range cases {
		if got, _, _ := cleanName(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
