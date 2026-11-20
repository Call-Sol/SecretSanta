package draws

import (
	"strings"
	"testing"
)

func TestFormatAssignmentEscapesNames(t *testing.T) {
	got := FormatAssignment(`Игра <1>`, `Анна & Боря`, "anna_santa", "до 2000 ₽", "2026-12-25")
	for _, part := range []string{
		"🎁 Твой жребий готов!",
		"Игра: Игра &lt;1&gt;",
		`Ты даришь подарок: <b>Анна &amp; Боря</b> (<a href="https://t.me/anna_santa">@anna_santa</a>)`,
		"Пожелания: до 2000 ₽",
		"Обмен: 25 декабря",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("message %q missing %s", got, part)
		}
	}
}

func TestFormatAssignmentSkipsEmptyDetails(t *testing.T) {
	got := FormatAssignment("Игра", "Анна", "", "", "")
	if strings.Contains(got, "Пожелания") || strings.Contains(got, "Обмен") {
		t.Fatalf("message %q", got)
	}
}
