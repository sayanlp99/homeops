package api

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func TestOccursOn(t *testing.T) {
	md := 31
	cases := []struct {
		name   string
		r      Recurring
		anchor string
		day    string
		want   bool
	}{
		{"daily every day", Recurring{Freq: "daily", Interval: 1}, "2026-10-01", "2026-10-07", true},
		{"daily every 3rd day hit", Recurring{Freq: "daily", Interval: 3}, "2026-10-01", "2026-10-07", true},
		{"daily every 3rd day miss", Recurring{Freq: "daily", Interval: 3}, "2026-10-01", "2026-10-08", false},
		{"before anchor", Recurring{Freq: "daily", Interval: 1}, "2026-10-10", "2026-10-09", false},
		{"weekly saturday", Recurring{Freq: "weekly", Interval: 1, Weekdays: []int{6}}, "2026-10-01", "2026-10-24", true},
		{"weekly wrong day", Recurring{Freq: "weekly", Interval: 1, Weekdays: []int{6}}, "2026-10-01", "2026-10-23", false},
		{"biweekly off week", Recurring{Freq: "weekly", Interval: 2, Weekdays: []int{6}}, "2026-10-01", "2026-10-10", false},
		{"biweekly on week", Recurring{Freq: "weekly", Interval: 2, Weekdays: []int{6}}, "2026-10-01", "2026-10-17", true},
		{"monthly 31st clamps to Feb 28", Recurring{Freq: "monthly", Interval: 1, MonthDay: &md}, "2026-01-01", "2027-02-28", true},
		{"monthly 31st on 30-day month", Recurring{Freq: "monthly", Interval: 1, MonthDay: &md}, "2026-01-01", "2026-11-30", true},
		{"monthly not that day", Recurring{Freq: "monthly", Interval: 1, MonthDay: &md}, "2026-01-01", "2026-11-29", false},
	}
	for _, c := range cases {
		if got := occursOn(&c.r, date(c.anchor), date(c.day)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestQuietHours(t *testing.T) {
	s, e := "22:00", "07:00"
	at := func(hm string) time.Time { tm, _ := time.Parse("15:04", hm); return tm }
	for hm, want := range map[string]bool{"21:59": false, "22:00": true, "02:00": true, "06:59": true, "07:00": false, "12:00": false} {
		if got := inQuietHours(&s, &e, at(hm)); got != want {
			t.Errorf("%s: got %v, want %v", hm, got, want)
		}
	}
	if inQuietHours(nil, &e, at("23:00")) {
		t.Error("nil start should disable quiet hours")
	}
}

func TestWeekStart(t *testing.T) {
	// 2026-10-22 is a Thursday.
	if got := weekStart(date("2026-10-22"), 1).Format(time.DateOnly); got != "2026-10-19" {
		t.Errorf("monday start: got %s", got)
	}
	if got := weekStart(date("2026-10-22"), 0).Format(time.DateOnly); got != "2026-10-18" {
		t.Errorf("sunday start: got %s", got)
	}
	if got := weekStart(date("2026-10-18"), 0).Format(time.DateOnly); got != "2026-10-18" {
		t.Errorf("same day: got %s", got)
	}
}
