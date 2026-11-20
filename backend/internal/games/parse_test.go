package games

import "testing"

func TestParseBudget(t *testing.T) {
	got, err := ParseBudget("  2\u00a0000 ")
	if err != nil || got == nil || *got != 2000 {
		t.Fatalf("budget %v err %v", got, err)
	}
	got, err = ParseBudget("-")
	if err != nil || got != nil {
		t.Fatalf("skip %v err %v", got, err)
	}
	if _, err := ParseBudget("0"); err != ErrInvalidBudget {
		t.Fatalf("zero %v", err)
	}
	if _, err := ParseBudget("20.5"); err != ErrInvalidBudget {
		t.Fatalf("decimal %v", err)
	}
}

func TestParseDay(t *testing.T) {
	day, skip, err := ParseDay("01.01.2030")
	if err != nil || skip {
		t.Fatal(err)
	}
	if got := EndOfDay(day).UTC().Format("2006-01-02T15:04:05Z"); got != "2030-01-01T21:00:00Z" {
		t.Fatalf("end %s", got)
	}
	if _, skip, err := ParseDay("—"); err != nil || !skip {
		t.Fatalf("skip %v %v", skip, err)
	}
	if _, _, err := ParseDay("31.02.2030"); err != ErrInvalidDate {
		t.Fatalf("bad day %v", err)
	}
}
