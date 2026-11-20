package games

import "testing"

func TestNormalizeDescription(t *testing.T) {
	got, err := NormalizeDescription("  до 2000 ₽, красный ")
	if err != nil || got != "до 2000 ₽, красный" {
		t.Fatalf("description %q err %v", got, err)
	}
	got, err = NormalizeDescription("-")
	if err != nil || got != "" {
		t.Fatalf("skip %q err %v", got, err)
	}
	if _, err := NormalizeDescription("  "); err != ErrInvalidDescription {
		t.Fatalf("blank %v", err)
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
