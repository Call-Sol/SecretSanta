package groups

import "testing"

func TestNormalizeName(t *testing.T) {
	got, err := NormalizeName("  Друзья  ")
	if err != nil || got != "Друзья" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizeName("   "); err != ErrInvalidName {
		t.Fatalf("blank err %v", err)
	}
	if _, err := NormalizeName(strings80()); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeName(strings80() + "я"); err != ErrInvalidName {
		t.Fatalf("long err %v", err)
	}
}

func strings80() string {
	return "йййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййййй"
}
