package telegram

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/games"
)

func gameCardScreen(view games.View, intro string) (string, Keyboard) {
	var b strings.Builder
	if intro != "" {
		b.WriteString(intro)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "🎁 %s\nСтатус: %s\nПодтвердили: %d / %d\nБюджет: %s\n%s\nОбмен: %s",
		view.Name, gameStatusLabel(view.Status), view.Confirmed, view.Participants,
		formatBudget(view.Budget), formatDeadline(view.Deadline), formatExchange(view.Exchange))
	switch view.Mine {
	case games.Pending:
		b.WriteString("\n\nТы ещё не подтвердил участие.")
	case games.Confirmed:
		b.WriteString("\n\nТы участвуешь.")
	case games.Declined:
		b.WriteString("\n\nТы отказался от участия.")
	}
	rows := make([][]Button, 0, 3)
	if registrationOpen(view) && view.Mine != "" {
		if view.Mine != games.Confirmed {
			rows = append(rows, []Button{{Text: "Подтвердить участие", Data: fmt.Sprintf("game:confirm:%d", view.ID)}})
		}
		if view.Mine != games.Declined {
			rows = append(rows, []Button{{Text: "Отказаться", Data: fmt.Sprintf("game:decline:%d", view.ID)}})
		}
	}
	rows = append(rows, []Button{{Text: "К группе", Data: fmt.Sprintf("grp:open:%d", view.GroupID)}})
	return b.String(), Keyboard{Rows: rows}
}

func registrationOpen(view games.View) bool {
	if view.Status != "REGISTRATION" {
		return false
	}
	return view.Deadline == nil || view.Deadline.After(time.Now())
}

func gameStatusLabel(status string) string {
	switch status {
	case "REGISTRATION":
		return "Регистрация"
	case "LOCKED":
		return "Регистрация закрыта"
	case "DRAWN":
		return "Жеребьёвка проведена"
	case "COMPLETED":
		return "Завершена"
	case "CANCELLED":
		return "Отменена"
	case "DRAFT":
		return "Черновик"
	default:
		return status
	}
}

func formatBudget(budget *int) string {
	if budget == nil {
		return "не задан"
	}
	raw := strconv.Itoa(*budget)
	var b strings.Builder
	for i, r := range raw {
		if i > 0 && (len(raw)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	b.WriteString(" ₽")
	return b.String()
}

func formatDeadline(deadline *time.Time) string {
	if deadline == nil {
		return "Регистрация: без срока"
	}
	return "Регистрация до: " + deadline.In(games.Moscow).Add(-time.Second).Format("02.01.2006")
}

func formatExchange(iso string) string {
	if iso == "" {
		return "дата не задана"
	}
	day, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return "дата не задана"
	}
	return day.Format("02.01.2006")
}
