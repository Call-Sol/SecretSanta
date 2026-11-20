package users

import "testing"

func TestProfileUsesFirstAndLastName(t *testing.T) {
	got := Profile(7, "@ann", " Анна ", "Иванова")
	if got.Username != "ann" || got.DisplayName != "Анна Иванова" || got.TelegramID != 7 {
		t.Fatalf("profile %+v", got)
	}
}

func TestProfileFallsBack(t *testing.T) {
	if got := Profile(1, "nick", "", ""); got.DisplayName != "nick" {
		t.Fatalf("username fallback %+v", got)
	}
	if got := Profile(1, "bad name", "", ""); got.Username != "" || got.DisplayName != "Участник" {
		t.Fatalf("empty fallback %+v", got)
	}
}
