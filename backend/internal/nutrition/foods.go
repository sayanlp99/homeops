package nutrition

// Per 100 g edible raw food:
// kcal, protein, fat, carbs, fibre, iron, calcium, vitamin C, vitamin A (µg RAE),
// folate (µg), B12 (µg), zinc, magnesium, potassium, sodium (mg).
type v = [15]float64

func n(x v) Nutrients {
	return Nutrients{Kcal: x[0], Protein: x[1], Fat: x[2], Carbs: x[3], Fibre: x[4], Iron: x[5], Calcium: x[6], VitC: x[7],
		VitA: x[8], Folate: x[9], B12: x[10], Zinc: x[11], Magnesium: x[12], Potassium: x[13], Sodium: x[14]}
}

type opt func(*Food)

func density(d float64) opt { return func(f *Food) { f.Density = d } }
func piece(g float64) opt   { return func(f *Food) { f.Piece = g } }
func edible(e float64) opt  { return func(f *Food) { f.Edible = e } }
func group(g string) opt    { return func(f *Food) { f.Group = g } }
func aka(a ...string) opt   { return func(f *Food) { f.aliases = append(f.aliases, a...) } }
func unit(u string, g float64) opt {
	return func(f *Food) {
		if f.Units == nil {
			f.Units = map[string]float64{}
		}
		f.Units[u] = g
	}
}

func food(key, name string, x v, opts ...opt) *Food {
	f := &Food{Key: key, Name: name, Per100: n(x)}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Typical densities (g/ml) for kitchen measures: 1 cup raw rice ≈ 200 g, atta ≈ 120 g,
// besan ≈ 90 g, oil ≈ 220 g; 1 tsp ground spice ≈ 2.5 g, salt ≈ 6 g.
const (
	dGrain  = 0.83
	dFlour  = 0.5
	dPowder = 0.5
	dSeed   = 0.55
	dChop   = 0.62 // chopped vegetables
	dLeafy  = 0.15
	dOil    = 0.92
)

var table = []*Food{
	// ---- grains, flours, breads ----
	food("rice", "Rice (white, raw)", v{356, 7.9, 0.5, 78.2, 2.8, 0.65, 7.5, 0, 0, 9, 0, 1.2, 19, 108, 2}, density(dGrain),
		aka("rice", "basmati", "basmati rice", "gobindobhog", "gobindobhog rice", "aatop", "aatop rice", "atop chal", "chal", "sona masoori", "rice raw", "parboiled rice", "sela rice", "minikit")),
	food("poha", "Poha (flattened rice)", v{346, 6.6, 1.2, 77.3, 2.0, 4.5, 9.4, 0, 0, 9, 0, 1.3, 31, 130, 7}, density(0.38), aka("poha", "flattened rice", "rice flakes", "chire", "chira", "chida", "beaten rice")),
	food("atta", "Whole wheat flour (atta)", v{321, 10.6, 1.5, 64.2, 11.4, 4.1, 31, 0, 0, 30, 0, 2.6, 125, 315, 2}, density(dFlour),
		aka("atta", "whole wheat flour", "wheat flour", "chapati flour", "aata", "gom", "gomer ata")),
	food("maida", "Refined flour (maida)", v{351, 10.4, 0.8, 74.3, 2.8, 1.8, 20, 0, 0, 15, 0, 0.8, 25, 120, 2}, density(dFlour), aka("maida", "all purpose flour", "refined flour", "moyda", "plain flour")),
	food("besan", "Gram flour (besan)", v{387, 22.4, 6.7, 57.8, 10.8, 4.9, 45, 0, 2, 437, 0, 2.8, 166, 846, 64}, density(0.38), aka("besan", "gram flour", "chickpea flour", "beshon")),
	food("suji", "Semolina (suji)", v{360, 12.7, 1.0, 72.8, 3.9, 1.2, 17, 0, 0, 72, 0, 1.05, 47, 186, 1}, density(0.7), aka("suji", "sooji", "rava", "rawa", "semolina")),
	food("oats", "Oats", v{379, 13.2, 6.5, 67.7, 10.1, 4.3, 52, 0, 0, 32, 0, 3.6, 138, 362, 6}, density(0.38), aka("oats", "rolled oats")),
	food("cornflour", "Cornflour", v{381, 0.3, 0.1, 91.3, 0.9, 0.5, 2, 0, 0, 0, 0, 0.1, 3, 3, 9}, density(dFlour), aka("cornflour", "corn flour", "cornstarch", "corn starch")),
	food("breadcrumbs", "Breadcrumbs", v{395, 13.4, 5.3, 71.9, 4.5, 4.8, 183, 0, 0, 100, 0, 1.5, 43, 196, 732}, density(0.45), aka("breadcrumbs", "bread crumbs", "panko")),
	food("bread", "Bread (white/brown)", v{265, 9.0, 3.2, 49, 2.7, 2.0, 80, 0, 0, 60, 0, 0.8, 23, 120, 480}, unit("slice", 28), piece(28),
		aka("bread", "white bread", "brown bread", "sandwich bread", "pauruti")),
	food("protein-bread", "Protein bread", v{265, 20, 6, 32, 8, 3.0, 60, 0, 0, 40, 0, 1.8, 70, 250, 420}, unit("slice", 33), piece(33), aka("protein bread", "high protein bread")),
	food("noodles", "Noodles (hakka, dry)", v{360, 11, 1.5, 74, 2.5, 1.3, 20, 0, 0, 18, 0, 0.9, 30, 150, 20}, aka("noodles", "hakka noodles", "chowmein noodles", "chow mein noodles")),
	food("batter", "Idli/dosa batter (ready-made)", v{150, 4.5, 0.6, 31, 1.8, 0.9, 15, 0, 0, 25, 0, 0.6, 20, 80, 350}, density(1.1),
		aka("idli batter", "dosa batter", "idli dosa batter", "batter idli")),
	food("naan", "Naan (ready-made)", v{290, 9, 6, 50, 2.2, 3.2, 75, 0, 0, 40, 0, 0.9, 22, 125, 460}, piece(90), aka("naan", "nan")),
	food("pizza-base", "Pizza base", v{270, 8.5, 4, 50, 2.5, 2.0, 30, 0, 0, 40, 0, 0.7, 20, 110, 450}, piece(120), aka("pizza base", "pizza bread")),
	food("papad", "Papad (raw)", v{371, 25.6, 3.3, 59.9, 18.6, 7.6, 143, 0, 0, 219, 0, 3.2, 271, 1000, 1745}, piece(12), aka("papad", "papadum", "papar", "pampad")),

	// ---- pulses ----
	food("moong-dal", "Moong dal (yellow, split)", v{333, 24.5, 1.2, 55.5, 8.2, 3.9, 75, 0, 3, 140, 0, 2.7, 127, 1150, 15}, density(dGrain),
		aka("moong dal", "mung dal", "moong", "yellow moong", "moong yellow", "sona moong", "mug dal", "muger dal", "moog dal", "split moong")),
	food("green-moong", "Green moong (whole)", v{334, 22.5, 1.1, 52, 17, 4.3, 92, 0, 6, 300, 0, 2.7, 127, 1150, 15}, density(dGrain),
		aka("green moong", "whole moong", "sabut moong", "moong green", "green gram", "mung beans", "moong whole", "green moong dal")),
	food("masoor-dal", "Masoor dal (red lentil)", v{343, 24.4, 0.8, 56, 10.5, 7.1, 50, 0, 2, 200, 0, 3.0, 82, 580, 6}, density(dGrain),
		aka("masoor dal", "masoor", "red lentil", "red lentils", "mushur dal", "musur dal", "mosur dal", "lentil", "lentils")),
	food("toor-dal", "Toor dal (arhar)", v{335, 21.7, 1.5, 55.2, 9.1, 3.9, 70, 0, 7, 150, 0, 2.8, 120, 1100, 17}, density(dGrain),
		aka("toor dal", "tur dal", "arhar dal", "toor", "arhar", "pigeon pea", "split pigeon peas")),
	food("chana-dal", "Chana dal", v{360, 21, 5.3, 55, 15, 5.3, 55, 0, 9, 170, 0, 3.0, 130, 800, 40}, density(dGrain),
		aka("chana dal", "chholar dal", "cholar dal", "bengal gram dal", "split chickpeas")),
	food("chana-whole", "Chickpeas, brown (kala chana)", v{360, 20, 5, 59, 17, 6.2, 150, 1, 3, 180, 0, 3.0, 140, 850, 30}, density(dGrain),
		aka("kala chana", "black chana", "brown chickpeas", "whole bengal gram", "chhola", "chola", "chholar", "chickpeas", "chickpea", "chana", "kabuli chana")),
	food("white-peas", "Dried white/yellow peas (ghugni)", v{340, 22, 1.4, 57, 15, 4.4, 55, 0, 5, 120, 0, 3.0, 115, 980, 15}, density(dGrain),
		aka("dried white peas", "white peas", "yellow peas", "dried peas", "motor", "matar dried", "safed matar", "ghugni peas", "vatana")),
	food("soya", "Soya chunks", v{345, 52, 0.5, 33, 13, 10, 350, 0, 0, 300, 0, 4.0, 290, 2400, 20}, density(0.2),
		aka("soya chunks", "soya", "soyabean", "soybean", "soya bean", "soya nuggets", "soya bori", "nutrela", "meal maker")),
	food("urad", "Urad dal / bori", v{341, 24, 1.6, 57, 11.9, 3.8, 70, 0, 2, 216, 0, 3.0, 180, 900, 40}, density(dGrain), piece(3),
		aka("urad dal", "urad", "biulir dal", "kalai dal", "bori", "boris", "badi", "vadi", "daaler bori", "sun dried bori")),

	// ---- vegetables ----
	food("potato", "Potato", v{77, 2, 0.1, 17, 2.2, 0.8, 12, 20, 0, 15, 0, 0.3, 23, 420, 6}, density(dChop), piece(150), aka("potato", "aloo", "alu", "aaloo", "baby potato")),
	food("onion", "Onion", v{40, 1.1, 0.1, 9.3, 1.7, 0.2, 23, 7.4, 0, 19, 0, 0.2, 10, 146, 4}, density(dChop), piece(110),
		aka("onion", "onions", "peyaj", "piyaj", "pyaj", "pyaz", "shallot", "red onion")),
	food("tomato", "Tomato", v{18, 0.9, 0.2, 3.9, 1.2, 0.3, 10, 14, 42, 15, 0, 0.2, 11, 237, 5}, density(dChop), piece(100), aka("tomato", "tamatar", "tometo", "tomato puree")),
	food("garlic", "Garlic", v{149, 6.4, 0.5, 33, 2.1, 1.7, 181, 31, 0, 3, 0, 1.2, 25, 401, 17}, density(0.6), unit("clove", 4), piece(40), aka("garlic", "rosun", "roshun", "lahsun", "garlic paste")),
	food("ginger", "Ginger", v{80, 1.8, 0.8, 18, 2, 0.6, 16, 5, 0, 11, 0, 0.3, 43, 415, 13}, density(0.6), unit("inch", 8), piece(8), aka("ginger", "ada", "adrak", "ginger paste", "grated ginger")),
	food("ginger-garlic", "Ginger-garlic paste", v{115, 4, 0.6, 25, 2, 1.1, 100, 18, 0, 7, 0, 0.8, 34, 408, 15}, density(1.0), aka("ginger garlic paste", "ginger-garlic paste", "adrak lahsun paste", "ada rosun bata")),
	food("green-chilli", "Green chilli", v{40, 2, 0.2, 9.5, 1.5, 1.0, 18, 111, 59, 23, 0, 0.3, 25, 340, 7}, piece(4), density(0.5),
		aka("green chilli", "green chillies", "green chili", "kancha lonka", "kacha lonka", "kancha lanka", "lonka", "hari mirch", "chilli", "chili", "chillies")),
	food("coriander-leaves", "Coriander leaves", v{23, 2.1, 0.5, 3.7, 2.8, 1.8, 67, 27, 337, 62, 0, 0.5, 26, 521, 46}, density(dLeafy), unit("bunch", 50), unit("sprig", 2), unit("handful", 15),
		aka("coriander leaves", "coriander", "dhania", "dhaniya", "cilantro", "dhonepata", "dhone pata", "coriander leaf", "fresh coriander")),
	food("mint", "Mint leaves", v{44, 3.3, 0.7, 8.4, 6.8, 5, 243, 31, 212, 114, 0, 1.1, 80, 569, 31}, density(dLeafy), unit("bunch", 40), unit("sprig", 2), unit("handful", 15), aka("mint", "pudina", "mint leaves")),
	food("curry-leaves", "Curry leaves", v{108, 6.1, 1, 18.7, 6.4, 0.9, 830, 4, 630, 90, 0, 0.2, 44, 300, 10}, density(0.1), unit("sprig", 1), unit("leaf", 0.1), aka("curry leaves", "curry leaf", "kadi patta", "kari pata")),
	food("okra", "Okra (bhindi, dherosh)", v{33, 1.9, 0.2, 7.5, 3.2, 0.6, 82, 23, 36, 60, 0, 0.6, 57, 299, 7}, density(dChop), piece(12),
		aka("okra", "bhindi", "ladies finger", "lady finger", "ladyfinger", "dherosh", "bhendi", "ladies fingers")),
	food("ridge-gourd", "Ridge gourd (jhinge)", v{20, 1.2, 0.2, 4.4, 2.0, 0.4, 20, 12, 20, 15, 0, 0.3, 14, 140, 3}, density(dChop), piece(150), aka("ridge gourd", "jhinge", "jhinga", "turai", "torai")),
	food("snake-gourd", "Snake gourd (chichinge)", v{18, 0.6, 0.3, 3.3, 1.0, 0.3, 26, 5, 9, 15, 0, 0.2, 15, 160, 3}, density(dChop), piece(200), aka("snake gourd", "chichinge", "chichinga", "chichingey", "padwal")),
	food("pointed-gourd", "Pointed gourd (potol)", v{20, 2.0, 0.3, 2.2, 3.0, 0.6, 30, 29, 10, 30, 0, 0.4, 9, 83, 3}, density(dChop), piece(50), aka("pointed gourd", "potol", "parwal", "patol", "parval")),
	food("bottle-gourd", "Bottle gourd (lau)", v{15, 0.6, 0, 3.4, 0.5, 0.2, 26, 10, 1, 6, 0, 0.7, 11, 150, 2}, density(dChop), piece(1000), aka("bottle gourd", "lau", "lauki", "louki", "dudhi", "ghia")),
	food("pumpkin", "Pumpkin (kumro)", v{26, 1.0, 0.1, 6.5, 0.5, 0.8, 21, 9, 426, 16, 0, 0.3, 12, 340, 1}, density(dChop), aka("pumpkin", "kumro", "kumra", "kaddu", "red pumpkin", "mishti kumro", "sweet pumpkin")),
	food("cauliflower", "Cauliflower", v{25, 1.9, 0.3, 5, 2, 0.4, 22, 48, 0, 57, 0, 0.3, 15, 299, 30}, density(0.45), piece(600), aka("cauliflower", "phulkopi", "fulkopi", "gobi", "phool gobi")),
	food("cabbage", "Cabbage", v{25, 1.3, 0.1, 5.8, 2.5, 0.5, 40, 36, 5, 43, 0, 0.2, 12, 170, 18}, density(0.35), piece(800), aka("cabbage", "badhakopi", "bandhakopi", "bandhakopir", "patta gobi")),
	food("green-peas", "Green peas (fresh/frozen)", v{81, 5.4, 0.4, 14.5, 5.1, 1.5, 25, 40, 38, 65, 0, 1.2, 33, 244, 5}, density(0.6), aka("green peas", "peas", "matar", "motorshuti", "koraishuti", "koraishutir", "koraishunti", "frozen peas")),
	food("spinach", "Spinach (palong)", v{23, 2.9, 0.4, 3.6, 2.2, 2.7, 99, 28, 469, 194, 0, 0.5, 79, 558, 79}, density(dLeafy), unit("bunch", 250),
		aka("spinach", "palak", "palong", "palong shaak", "palong shak", "palang shak", "palong saag")),
	food("brinjal", "Brinjal (begun)", v{25, 1.0, 0.2, 5.9, 3.0, 0.3, 9, 2.2, 1, 22, 0, 0.2, 14, 229, 2}, density(dChop), piece(250), aka("brinjal", "begun", "baingan", "eggplant", "aubergine")),
	food("bitter-gourd", "Bitter gourd (korola/uchhe)", v{17, 1.0, 0.2, 3.7, 2.8, 0.4, 19, 84, 24, 72, 0, 0.8, 17, 296, 5}, density(dChop), piece(100),
		aka("bitter gourd", "karela", "korola", "korala", "uchhe", "ucche", "bitter melon")),
	food("raw-banana", "Raw banana (kanchkola)", v{90, 1.3, 0.3, 22, 2.6, 0.6, 5, 18, 3, 20, 0, 0.15, 37, 400, 1}, piece(120), edible(0.8),
		aka("raw banana", "kanchkola", "kachkola", "kancha kola", "green banana", "plantain", "kaccha kela")),
	food("jackfruit-raw", "Raw jackfruit (echor)", v{51, 2.6, 0.3, 9.4, 3.6, 0.5, 30, 12, 3, 20, 0, 0.4, 27, 300, 3}, density(dChop), aka("raw jackfruit", "echor", "enchor", "young jackfruit", "kathal")),
	food("banana-flower", "Banana flower (mocha)", v{40, 1.6, 0.6, 7, 5.7, 1.0, 30, 5, 3, 20, 0, 0.4, 48, 553, 13}, density(0.4), piece(240), edible(0.6), aka("banana flower", "banana blossom", "mocha", "mochar", "kele ka phool")),
	food("spring-onion", "Spring onion / peyajkoli", v{32, 1.8, 0.2, 7.3, 2.6, 1.5, 72, 19, 50, 64, 0, 0.4, 20, 276, 16}, density(0.3), unit("bunch", 150),
		aka("spring onion", "spring onions", "green onion", "scallion", "peyajkoli", "peyaj koli", "pyajkoli", "onion stalks", "onion flowers")),
	food("carrot", "Carrot", v{41, 0.9, 0.2, 9.6, 2.8, 0.3, 33, 6, 835, 19, 0, 0.2, 12, 320, 69}, density(dChop), piece(70), aka("carrot", "gajar", "gajor")),
	food("beans", "French beans", v{31, 1.8, 0.2, 7, 2.7, 1.0, 37, 12, 35, 33, 0, 0.2, 25, 211, 6}, density(0.5), aka("french beans", "beans", "green beans", "bins", "string beans")),
	food("capsicum", "Capsicum", v{20, 0.9, 0.2, 4.6, 1.7, 0.3, 10, 80, 18, 10, 0, 0.1, 10, 175, 3}, density(0.45), piece(120), aka("capsicum", "bell pepper", "green capsicum", "shimla mirch")),
	food("cucumber", "Cucumber", v{15, 0.7, 0.1, 3.6, 0.5, 0.3, 16, 3, 5, 7, 0, 0.2, 13, 147, 2}, density(dChop), piece(200), aka("cucumber", "shosha", "sasha", "kheera", "khira")),
	food("sweet-corn", "Sweet corn", v{86, 3.3, 1.4, 18.7, 2, 0.5, 2, 6.8, 9, 42, 0, 0.5, 37, 270, 15}, density(0.6), piece(100), // a cob gives ~100 g kernels
		aka("sweet corn", "corn", "corn kernels", "sweet corn kernels", "corn on the cob", "bhutta", "makai", "bhutta dana")),
	food("radish", "Radish (mulo)", v{16, 0.7, 0.1, 3.4, 1.6, 0.3, 25, 15, 0, 25, 0, 0.3, 10, 233, 39}, density(dChop), piece(200), aka("radish", "mulo", "mooli", "white radish")),
	food("sweet-potato", "Sweet potato", v{86, 1.6, 0.1, 20, 3, 0.6, 30, 2.4, 709, 11, 0, 0.3, 25, 337, 55}, density(dChop), piece(200), aka("sweet potato", "ranga alu", "shakarkandi", "rangalu")),
	food("drumstick", "Drumstick (sajne data)", v{37, 2.1, 0.2, 8.5, 3.2, 0.4, 30, 141, 4, 44, 0, 0.5, 45, 461, 42}, piece(60), aka("drumstick", "sajne data", "sojne data", "moringa pods", "drumsticks")),
	food("lemon", "Lemon/lime juice", v{22, 0.4, 0.2, 6.9, 0.3, 0.1, 6, 39, 1, 20, 0, 0.05, 6, 103, 1}, density(1.0), piece(15),
		aka("lemon", "lime", "lemon juice", "lime juice", "lebu", "nimbu", "gondhoraj lebu", "lebur rosh")),
	food("mushroom", "Mushroom", v{22, 3.1, 0.3, 3.3, 1, 0.5, 3, 2, 0, 17, 0.04, 0.5, 9, 318, 5}, density(0.4), aka("mushroom", "button mushroom")),
	food("lettuce", "Salad greens", v{15, 1.4, 0.2, 2.9, 1.3, 0.9, 36, 9, 370, 38, 0, 0.2, 13, 194, 28}, density(dLeafy), aka("lettuce", "salad leaves", "salad greens")),
	food("beetroot", "Beetroot", v{43, 1.6, 0.2, 9.6, 2.8, 0.8, 16, 4.9, 2, 109, 0, 0.35, 23, 325, 78}, density(dChop), piece(150), aka("beetroot", "beet", "bit")),

	// ---- nuts, seeds, coconut ----
	food("coconut", "Coconut (fresh, grated)", v{354, 3.3, 33.5, 15.2, 9.0, 2.4, 14, 3.3, 0, 26, 0, 1.1, 32, 356, 20}, density(0.35),
		aka("coconut", "grated coconut", "narkel", "narkol", "fresh coconut", "nariyal", "desiccated coconut")),
	food("peanut", "Peanuts", v{567, 25.8, 49.2, 16.1, 8.5, 4.6, 92, 0, 0, 240, 0, 3.3, 168, 705, 18}, density(0.6), aka("peanut", "peanuts", "groundnut", "groundnuts", "badam", "chinebadam", "moongphali")),
	food("poppy", "Poppy seeds (posto)", v{525, 18, 42, 28, 19.5, 9.8, 1438, 1, 0, 82, 0, 7.9, 347, 719, 26}, density(dSeed), aka("poppy seeds", "poppy seed", "posto", "khus khus", "khas khas", "posto bata", "poppy")),
	food("mustard-seed", "Mustard seeds", v{508, 26, 36, 28, 12, 9.2, 266, 7, 2, 162, 0, 6.1, 370, 738, 13}, density(dSeed),
		aka("mustard seeds", "mustard seed", "mustard", "shorshe", "sorshe", "sarson", "rai", "mustard paste", "kasundi", "shorshe bata", "black mustard", "yellow mustard")),
	food("cashew", "Cashew nuts", v{553, 18, 44, 30, 3.3, 6.7, 37, 0.5, 0, 25, 0, 5.8, 292, 660, 12}, density(0.55), piece(1.5), aka("cashew", "cashews", "kaju", "cashew nuts", "kaju badam")),
	food("raisin", "Raisins", v{299, 3.1, 0.5, 79, 3.7, 1.9, 50, 2.3, 0, 5, 0, 0.2, 32, 749, 11}, density(0.6), aka("raisins", "raisin", "kishmish")),
	food("almond", "Almonds", v{579, 21, 50, 21.6, 12.5, 3.7, 269, 0, 0, 44, 0, 3.1, 270, 733, 1}, density(0.6), piece(1.2), aka("almond", "almonds", "kath badam")),
	food("makhana", "Makhana (fox nuts)", v{347, 9.7, 0.1, 76.9, 14.5, 1.4, 60, 0, 0, 14, 0, 1.4, 67, 500, 1}, density(0.1), aka("makhana", "fox nut", "fox nuts", "lotus seeds")),
	food("sesame", "Sesame seeds", v{573, 17.7, 49.7, 23.4, 11.8, 14.6, 975, 0, 0, 97, 0, 7.8, 351, 468, 11}, density(dSeed), aka("sesame", "sesame seeds", "til")),

	// ---- spices, salt, sugar ----
	food("turmeric", "Turmeric powder", v{312, 9.7, 3.3, 67, 22.7, 41, 168, 0.7, 0, 20, 0, 4.5, 208, 2080, 27}, density(dPowder), aka("turmeric", "haldi", "holud", "turmeric powder", "holud gura")),
	food("cumin", "Cumin (jeera)", v{375, 17.8, 22.3, 44.2, 10.5, 66, 931, 7.7, 64, 10, 0, 4.8, 366, 1788, 168}, density(0.45),
		aka("cumin", "jeera", "jeere", "jire", "jira", "cumin seeds", "cumin powder", "roasted cumin powder", "bhaja moshla", "jeera powder")),
	food("coriander-powder", "Coriander powder", v{298, 12.4, 17.8, 55, 41.9, 16.3, 709, 21, 0, 0, 0, 4.7, 330, 1267, 35}, density(dPowder), aka("coriander powder", "dhania powder", "dhone gura", "coriander seeds", "dhone")),
	food("chilli-powder", "Red chilli powder", v{318, 12, 17.3, 56.6, 27.2, 7.8, 148, 76, 2081, 106, 0, 2.5, 152, 2014, 30}, density(dPowder), piece(0.5),
		aka("red chilli powder", "chilli powder", "chili powder", "kashmiri chilli powder", "kashmiri", "lal mirch", "lonka gura", "shukno lonka", "dry red chilli", "dried red chilli", "red chilli", "kashmiri mirch", "chilli flakes")),
	food("spice-mix", "Whole & ground spices", v{350, 12, 15, 50, 30, 20, 400, 5, 20, 50, 0, 3, 250, 1500, 50}, density(dPowder), unit("pod", 0.3), unit("stick", 2), unit("leaf", 0.3), piece(0.3),
		aka("garam masala", "gorom moshla", "garam moshla", "cardamom", "elaichi", "elach", "cloves", "clove", "laung", "lobongo", "cinnamon", "dalchini", "darchini", "bay leaf", "bay leaves", "tej patta", "tejpata",
			"panch phoron", "panch phoran", "kalonji", "kalo jeere", "kalo jire", "nigella", "fenugreek seeds", "methi seeds", "methi", "fennel", "mouri", "saunf", "radhuni", "ajwain", "jowan", "hing", "asafoetida",
			"kasuri methi", "chaat masala", "meat masala", "chicken masala", "kitchen king", "black pepper", "pepper", "peppercorns", "white pepper", "star anise", "nutmeg", "mace", "javitri", "jaifal", "amchur", "shahi jeera", "black salt", "biryani masala", "sambar powder", "pav bhaji masala", "kebab chini", "sabzi masala", "oregano", "mixed herbs")),
	food("peri-peri", "Peri peri seasoning", v{280, 10, 8, 45, 20, 10, 200, 5, 1000, 40, 0, 2, 120, 1200, 15000}, density(dPowder), aka("peri peri", "piri piri", "peri peri masala", "peri peri seasoning")),
	food("salt", "Salt", v{0, 0, 0, 0, 0, 0.3, 24, 0, 0, 0, 0, 0.1, 1, 8, 38758}, density(1.2), aka("salt", "noon", "lobon", "namak", "rock salt", "sendha namak")),
	food("sugar", "Sugar", v{387, 0, 0, 100, 0, 0.05, 1, 0, 0, 0, 0, 0, 0, 2, 1}, density(0.85), aka("sugar", "chini", "cheeni")),
	food("jaggery", "Jaggery", v{383, 0.4, 0.1, 95, 0, 4.6, 80, 0, 0, 1, 0, 0.2, 70, 450, 30}, density(0.8), aka("jaggery", "gur", "gud", "nolen gur", "patali gur")),
	food("tamarind", "Tamarind", v{239, 2.8, 0.6, 62.5, 5.1, 2.8, 74, 3.5, 2, 14, 0, 0.1, 92, 628, 28}, density(0.8), aka("tamarind", "tetul", "imli")),

	// ---- oils & fats ----
	food("oil", "Cooking oil", v{884, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, density(dOil), group("oil"),
		aka("oil", "mustard oil", "shorsher tel", "sorsher tel", "refined oil", "sunflower oil", "vegetable oil", "rice bran oil", "groundnut oil", "olive oil", "coconut oil", "tel", "cooking oil")),
	food("ghee", "Ghee", v{900, 0, 99.5, 0, 0, 0, 4, 0, 684, 0, 0, 0, 0, 5, 2}, density(0.91), group("dairy"), aka("ghee", "ghi", "clarified butter")),
	food("butter", "Butter", v{717, 0.9, 81, 0.1, 0, 0, 24, 0, 684, 3, 0.17, 0.1, 2, 24, 600}, density(0.95), group("dairy"), aka("butter", "makhan", "amul butter")),

	// ---- dairy & eggs ----
	food("milk", "Milk (toned)", v{58, 3.1, 3.0, 4.7, 0, 0.1, 113, 1, 30, 5, 0.45, 0.4, 11, 140, 45}, density(1.03), group("dairy"), aka("milk", "dudh", "doodh", "toned milk", "full cream milk")),
	food("curd", "Curd (dahi)", v{61, 3.5, 3.3, 4.7, 0, 0.1, 121, 0.5, 27, 7, 0.37, 0.6, 12, 155, 46}, density(1.03), group("dairy"),
		aka("curd", "dahi", "doi", "yogurt", "yoghurt", "tok doi", "hung curd", "greek yogurt", "plain curd")),
	food("paneer", "Paneer", v{265, 18.3, 20.8, 1.2, 0, 0.2, 480, 0, 120, 10, 0.7, 2.7, 25, 100, 30}, density(0.5), group("dairy"), aka("paneer", "cottage cheese", "panir")),
	food("chhana", "Chhana (fresh)", v{225, 15, 17, 3, 0, 0.2, 300, 0, 100, 10, 0.6, 2, 20, 100, 30}, density(0.5), group("dairy"), aka("chhana", "chana cheese", "chenna", "chhena", "channa", "chanar")),
	food("cheese", "Cheese (processed)", v{315, 20, 25, 2, 0, 0.2, 600, 0, 230, 10, 1.0, 3, 25, 130, 1200}, density(0.45), unit("slice", 20), unit("cube", 20), piece(20), group("dairy"),
		aka("cheese", "cheese slice", "cheese slices", "processed cheese", "cheddar", "cheese cube")),
	food("mozzarella", "Mozzarella", v{300, 22, 22, 2.2, 0, 0.4, 505, 0, 179, 7, 2.3, 2.9, 20, 76, 627}, density(0.45), group("dairy"), aka("mozzarella", "pizza cheese")),
	food("cream", "Fresh cream", v{195, 2.7, 19, 3.7, 0, 0.1, 96, 0.6, 168, 4, 0.2, 0.3, 9, 132, 40}, density(1.0), group("dairy"), aka("cream", "fresh cream", "malai")),
	food("egg", "Egg", v{143, 12.6, 9.5, 0.7, 0, 1.75, 56, 0, 160, 47, 0.89, 1.3, 12, 138, 142}, piece(50), group("egg"),
		aka("egg", "eggs", "dim", "deem", "anda", "boiled egg", "boiled eggs", "hen egg", "hen eggs", "chicken egg", "chicken eggs", "duck egg", "duck eggs",
			"duck or chicken eggs", "hasher dim", "haser dim")),
	food("egg-white", "Egg white", v{52, 10.9, 0.7, 0.2, 0, 0.08, 7, 0, 0, 4, 0.09, 0.03, 11, 163, 166}, piece(33), group("egg"),
		aka("egg white", "egg whites", "egg-white", "whites of eggs")),

	// ---- meat & fish ----
	food("chicken", "Chicken (curry cut, with bone)", v{119, 19.9, 3.9, 0, 0, 0.9, 10, 0, 18, 8, 0.5, 1.8, 23, 260, 85}, edible(0.75), group("chicken"),
		aka("chicken", "murgi", "murgir mangsho", "chicken curry cut", "curry cut chicken", "chicken legs", "chicken drumsticks", "chicken thighs", "chicken with bone", "country chicken")),
	food("chicken-boneless", "Chicken (boneless, breast)", v{120, 22.5, 2.6, 0, 0, 0.4, 5, 0, 9, 4, 0.2, 0.7, 28, 334, 60}, group("chicken"),
		aka("chicken breast", "boneless chicken", "chicken boneless", "chicken fillet", "chicken tenders", "boneless chicken breast", "chicken cubes")),
	food("chicken-mince", "Chicken mince", v{143, 17.4, 8.1, 0, 0, 0.8, 6, 0, 26, 4, 0.6, 1.5, 21, 250, 60}, group("chicken"), aka("chicken mince", "chicken keema", "minced chicken", "ground chicken", "keema")),
	food("fish", "Fish, rohu/katla (with bone)", v{111, 17.5, 4.5, 0, 0, 1.0, 120, 0, 50, 15, 2.5, 0.9, 25, 290, 50}, edible(0.85), group("fish"),
		aka("fish", "rohu", "rui", "rui maach", "rui mach", "katla", "catla", "macher", "machher", "maacher", "maach", "mach", "machh", "fish pieces", "fish steaks", "pabda", "tangra", "parshe", "mrigal", "bhetki", "bekti", "basa", "pomfret", "fish curry cut")),
	food("hilsa", "Hilsa (ilish, with bone)", v{273, 21.8, 19.4, 0, 0, 2.1, 180, 0, 69, 10, 3.0, 1.0, 30, 350, 80}, edible(0.75), group("fish"),
		aka("ilish", "hilsa", "ilish maach", "ilish mach", "elish")),
	food("prawn", "Prawns (peeled)", v{85, 20.1, 0.5, 0, 0, 0.5, 64, 0, 0, 19, 1.1, 1.0, 22, 113, 119}, group("seafood"),
		aka("prawn", "prawns", "shrimp", "shrimps", "chingri", "golda chingri", "bagda chingri", "kucho chingri", "tiger prawns")),

	// ---- fruit ----
	food("banana", "Banana", v{89, 1.1, 0.3, 22.8, 2.6, 0.3, 5, 8.7, 3, 20, 0, 0.15, 27, 358, 1}, piece(100), group("fruit"), aka("banana", "kola", "kela", "ripe banana")),
	food("apple", "Apple", v{52, 0.3, 0.2, 13.8, 2.4, 0.1, 6, 4.6, 3, 3, 0, 0.04, 5, 107, 1}, piece(150), group("fruit"), aka("apple", "apel")),
	food("papaya", "Papaya", v{43, 0.5, 0.3, 10.8, 1.7, 0.25, 20, 61, 47, 37, 0, 0.08, 21, 182, 8}, density(0.6), piece(150), group("fruit"), aka("papaya", "pepe", "papita", "ripe papaya")),
	food("guava", "Guava", v{68, 2.6, 1.0, 14.3, 5.4, 0.26, 18, 228, 31, 49, 0, 0.23, 22, 417, 2}, piece(100), group("fruit"), aka("guava", "peyara", "amrood")),
	food("orange", "Orange (mandarin)", v{53, 0.8, 0.3, 13.3, 1.8, 0.15, 37, 27, 34, 16, 0, 0.07, 12, 166, 2}, piece(100), group("fruit"), aka("orange", "kamla lebu", "kamala lebu", "mandarin", "santra", "kinnow")),
	food("sweet-lime", "Sweet lime (mosambi)", v{43, 0.8, 0.3, 9.3, 1.0, 0.7, 40, 50, 2, 20, 0, 0.1, 10, 160, 2}, piece(150), group("fruit"), aka("sweet lime", "mosambi", "musambi", "musambi lebu")),
	food("pomegranate", "Pomegranate", v{83, 1.7, 1.2, 18.7, 4, 0.3, 10, 10, 0, 38, 0, 0.35, 12, 236, 3}, density(0.6), piece(150), group("fruit"), aka("pomegranate", "bedana", "dalim", "anar")),
	food("watermelon", "Watermelon", v{30, 0.6, 0.15, 7.6, 0.4, 0.24, 7, 8, 28, 3, 0, 0.1, 10, 112, 1}, density(0.6), group("fruit"), aka("watermelon", "tormuj", "tarbooj")),
	food("mango", "Mango", v{60, 0.8, 0.4, 15, 1.6, 0.16, 11, 36, 54, 43, 0, 0.09, 10, 168, 1}, density(0.65), piece(200), group("fruit"), aka("mango", "aam", "ripe mango")),
	food("litchi", "Litchi", v{66, 0.8, 0.4, 16.5, 1.3, 0.3, 5, 71.5, 0, 14, 0, 0.07, 10, 171, 1}, piece(10), group("fruit"), aka("litchi", "lychee", "lichu")),
	food("pear", "Pear (nashpati)", v{57, 0.4, 0.1, 15.2, 3.1, 0.2, 9, 4.3, 1, 7, 0, 0.1, 7, 116, 1}, piece(150), group("fruit"), aka("pear", "nashpati", "naspati")),
	food("pineapple", "Pineapple", v{50, 0.5, 0.1, 13.1, 1.4, 0.3, 13, 47.8, 3, 18, 0, 0.12, 12, 109, 1}, density(0.65), group("fruit"), aka("pineapple", "anaras", "ananas")),
	food("jamun", "Jamun (kalo jaam)", v{60, 0.7, 0.2, 15.6, 0.6, 0.2, 19, 14, 3, 5, 0, 0.1, 15, 79, 14}, piece(5), group("fruit"), aka("jamun", "kalo jaam", "jam fruit", "black plum")),
	food("sapota", "Sapota (chikoo)", v{83, 0.4, 1.1, 20, 5.3, 0.8, 21, 14.7, 3, 14, 0, 0.1, 12, 193, 12}, piece(100), group("fruit"), aka("sapota", "chikoo", "chiku", "sobeda")),
	food("custard-apple", "Custard apple (ata)", v{94, 2.1, 0.3, 23.6, 4.4, 0.6, 30, 36, 1, 14, 0, 0.1, 18, 382, 4}, piece(120), group("fruit"), aka("custard apple", "sitaphal", "ata fruit", "aata phal")),
	food("dates", "Dates", v{282, 2.5, 0.4, 75, 8, 1.0, 39, 0.4, 1, 19, 0, 0.3, 43, 656, 2}, piece(8), group("fruit"), aka("dates", "khejur", "khajur")),

	// ---- condiments ----
	food("pickle", "Pickle (oil-based)", v{230, 1.5, 20, 10, 3, 2.0, 50, 5, 20, 10, 0, 0.3, 30, 200, 2800}, density(0.9), aka("pickle", "achar", "aachar", "mango pickle", "lime pickle")),
	food("soy-sauce", "Soy sauce", v{53, 8.1, 0.6, 4.9, 0.8, 1.5, 33, 0, 0, 18, 0, 0.4, 43, 435, 5500}, density(1.1), aka("soy sauce", "soya sauce", "dark soy sauce", "light soy sauce")),
	food("sauce", "Chilli sauce / ketchup", v{112, 1.3, 0.2, 26, 0.3, 0.4, 15, 4, 25, 10, 0, 0.2, 13, 280, 900}, density(1.1),
		aka("ketchup", "tomato ketchup", "tomato sauce", "chilli sauce", "green chilli sauce", "red chilli sauce", "schezwan sauce", "sauce", "pizza sauce")),
	food("vinegar", "Vinegar", v{18, 0, 0, 0.04, 0, 0, 6, 0, 0, 0, 0, 0, 1, 2, 2}, density(1.0), aka("vinegar")),
	food("mayonnaise", "Mayonnaise", v{680, 1, 75, 0.6, 0, 0.2, 8, 0, 20, 5, 0.1, 0.1, 1, 20, 635}, density(0.95), aka("mayonnaise", "mayo")),
	food("honey", "Honey", v{304, 0.3, 0, 82.4, 0.2, 0.4, 6, 0.5, 0, 2, 0, 0.2, 2, 52, 4}, density(1.4), aka("honey", "modhu")),
	food("peanut-butter", "Peanut butter", v{588, 25, 50, 20, 6, 1.9, 43, 0, 0, 87, 0, 2.5, 154, 649, 430}, density(1.05), aka("peanut butter")),

	// Things that carry no nutrition worth counting.
	food("water", "Water", v{}, density(1.0), aka("water", "jol", "pani", "hot water", "warm water", "ice")),
}
