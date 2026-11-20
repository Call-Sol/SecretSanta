package draws

import (
	"html"
	"strconv"
	"strings"
	"time"
)

var ruMonth = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// Mine is one person's own receiver. It never includes anyone else's pair.
type Mine struct {
	GameID       int64
	GroupID      int64
	GameName     string
	ReceiverName string
	Description  string
	Exchange     string
}

// AssignmentPayload is the private outbox body for one giver.
type AssignmentPayload struct {
	GameName     string `json:"game_name"`
	ReceiverName string `json:"receiver_name"`
	Description  string `json:"description,omitempty"`
	ExchangeOn   string `json:"exchange_on,omitempty"`
}

// FormatAssignment renders the private draw message as Telegram HTML.
func FormatAssignment(gameName, receiver, description, exchange string) string {
	var b strings.Builder
	b.WriteString("🎁 Твой жребий готов!\n")
	b.WriteString("Игра: ")
	b.WriteString(html.EscapeString(gameName))
	b.WriteString("\nТы даришь подарок: <b>")
	b.WriteString(html.EscapeString(receiver))
	b.WriteString("</b>")
	if description != "" {
		b.WriteString("\nПожелания: ")
		b.WriteString(html.EscapeString(description))
	}
	if day := formatExchange(exchange); day != "" {
		b.WriteString("\nОбмен: ")
		b.WriteString(day)
	}
	return b.String()
}

func formatExchange(iso string) string {
	if iso == "" {
		return ""
	}
	day, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return ""
	}
	return strconv.Itoa(day.Day()) + " " + ruMonth[day.Month()-1]
}
