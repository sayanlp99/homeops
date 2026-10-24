package nutrition

import "math"

// Profile is what a person tells us about themselves.
type Profile struct {
	Sex      string  `json:"sex"`      // "female" | "male"
	Age      int     `json:"age"`      // years
	HeightCm float64 `json:"heightCm"` //
	WeightKg float64 `json:"weightKg"` //
	Activity string  `json:"activity"` // sedentary | light | moderate | active
	Goal     string  `json:"goal"`     // lose | maintain | gain
}

var activityFactor = map[string]float64{"sedentary": 1.2, "light": 1.375, "moderate": 1.55, "active": 1.725}

// Valid reports whether the profile has enough to compute targets.
func (p Profile) Valid() bool {
	return (p.Sex == "female" || p.Sex == "male") && p.Age >= 14 && p.Age <= 100 &&
		p.HeightCm >= 100 && p.HeightCm <= 230 && p.WeightKg >= 25 && p.WeightKg <= 250
}

// Targets are daily goals. Sodium is a ceiling, Fat a ceiling (≤30% of energy); the
// rest are amounts to reach.
type Targets struct {
	Nutrients
	BMR  float64 `json:"bmr"`
	TDEE float64 `json:"tdee"` // energy spent per day at this activity level
}

// DailyTargets computes energy from Mifflin-St Jeor × activity, then adjusts for the
// goal: a gentle deficit (~15%, at most 400 kcal, never below BMR) for weight loss, a
// small surplus for gain. Protein: 1.1 g/kg to maintain (above the 0.83 g/kg RDA, as Indian diets run low), 1.2 g/kg while losing (keeps
// muscle and fullness), 1.4 g/kg to gain. Micronutrients: ICMR-NIN 2020 RDAs for
// sedentary Indian adults.
func DailyTargets(p Profile) Targets {
	bmr := 10*p.WeightKg + 6.25*p.HeightCm - 5*float64(p.Age)
	if p.Sex == "male" {
		bmr += 5
	} else {
		bmr -= 161
	}
	f := activityFactor[p.Activity]
	if f == 0 {
		f = 1.375
	}
	tdee := bmr * f
	kcal := tdee
	protein := 1.1
	switch p.Goal {
	case "lose":
		kcal = math.Max(tdee-math.Min(tdee*0.15, 400), bmr)
		protein = 1.2
	case "gain":
		kcal = tdee + 250
		protein = 1.4
	}
	t := Targets{BMR: math.Round(bmr), TDEE: math.Round(tdee)}
	t.Kcal = math.Round(kcal/10) * 10
	t.Protein = math.Round(p.WeightKg * protein)
	t.Fat = math.Round(t.Kcal * 0.30 / 9)                          // ceiling
	t.Carbs = math.Round((t.Kcal - t.Protein*4 - t.Kcal*0.27) / 4) // the rest, at ~27% energy from fat
	t.Fibre = math.Round(math.Max(25, t.Kcal/1000*14))
	t.Calcium = 1000
	t.B12 = 2.5
	t.Potassium = 3500
	t.Sodium = 2000 // ceiling (WHO: < 5 g salt)
	if p.Sex == "male" {
		t.Iron, t.VitC, t.VitA, t.Folate, t.Zinc, t.Magnesium = 19, 80, 1000, 300, 17, 440
	} else {
		t.Iron, t.VitC, t.VitA, t.Folate, t.Zinc, t.Magnesium = 29, 65, 840, 220, 13.2, 370
	}
	return t
}

// Limits are nutrients where the target is a ceiling, not a goal.
var Limits = map[string]bool{"fat": true, "sodium": true}
