package telegram

import (
	"fmt"
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
	fmt.Fprintf(&b, "🎁 %s\nСтатус: %s\nПодтвердили: %d / %d\nОписание: %s\n%s\nОбмен: %s",
		view.Name, gameStatusLabel(view.Status), view.Confirmed, view.Participants,
		formatDescription(view.Description), formatDeadline(view.Deadline), formatExchange(view.Exchange))
	switch view.Mine {
	case games.Pending:
		b.WriteString("\n\nТы ещё не подтвердил участие.")
	case games.Confirmed:
		b.WriteString("\n\nТы участвуешь.")
	case games.Declined:
		b.WriteString("\n\nТы отказался от участия.")
	}
	rows := make([][]Button, 0, 4)
	if registrationOpen(view) && view.Mine != "" {
		if view.Mine != games.Confirmed {
			rows = append(rows, []Button{{Text: "Подтвердить участие", Data: fmt.Sprintf("game:confirm:%d", view.ID)}})
		}
		if view.Mine != games.Declined {
			rows = append(rows, []Button{{Text: "Отказаться", Data: fmt.Sprintf("game:decline:%d", view.ID)}})
		}
	}
	if view.Role == "OWNER" || view.Role == "ADMIN" {
		rows = append(rows, []Button{{Text: "Список участников", Data: fmt.Sprintf("game:people:%d", view.ID)}})
	}
	rows = append(rows, []Button{{Text: "К группе", Data: fmt.Sprintf("grp:open:%d", view.GroupID)}})
	return b.String(), Keyboard{Rows: rows}
}

func gamePeopleScreen(view games.View, people []games.Participant) (string, Keyboard) {
	groups := map[string][]string{
		"confirmed": nil,
		"pending":   nil,
		"declined":  nil,
		"waiting":   nil,
	}
	for _, person := range people {
		switch {
		case person.AssignmentState == "AWAITING_ASSIGNMENT":
			groups["waiting"] = append(groups["waiting"], person.DisplayName)
		case person.Status == games.Confirmed:
			groups["confirmed"] = append(groups["confirmed"], person.DisplayName)
		case person.Status == games.Pending:
			groups["pending"] = append(groups["pending"], person.DisplayName)
		case person.Status == games.Declined:
			groups["declined"] = append(groups["declined"], person.DisplayName)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Участники «%s»", view.Name)
	for _, section := range []struct {
		title string
		key   string
	}{
		{"Подтвердили", "confirmed"},
		{"Ожидают решения", "pending"},
		{"Отказались", "declined"},
		{"Ожидают включения", "waiting"},
	} {
		fmt.Fprintf(&b, "\n\n%s:", section.title)
		names := groups[section.key]
		if len(names) == 0 {
			b.WriteString("\n• нет")
			continue
		}
		for _, name := range names {
			fmt.Fprintf(&b, "\n• %s", name)
		}
	}
	return b.String(), Keyboard{Rows: [][]Button{
		{{Text: "К игре", Data: fmt.Sprintf("game:open:%d", view.ID)}},
	}}
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

func formatDescription(description string) string {
	if description == "" {
		return "не задано"
	}
	return description
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
