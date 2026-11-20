package telegram

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
)

func groupListScreen(items []groups.Summary) (string, Keyboard) {
	if len(items) == 0 {
		return noGroupsText, Keyboard{Rows: [][]Button{
			{{Text: "Создать группу", Data: "grp:new"}},
			{{Text: "В меню", Data: "menu:home"}},
		}}
	}
	rows := make([][]Button, 0, len(items)+2)
	var b strings.Builder
	b.WriteString("Твои группы:")
	for _, item := range items {
		fmt.Fprintf(&b, "\n• %s", item.Name)
		rows = append(rows, []Button{{Text: buttonLabel(item.Name), Data: fmt.Sprintf("grp:open:%d", item.ID)}})
	}
	rows = append(rows,
		[]Button{{Text: "Создать группу", Data: "grp:new"}},
		[]Button{{Text: "В меню", Data: "menu:home"}},
	)
	return b.String(), Keyboard{Rows: rows}
}

func groupCardScreen(card groups.Card, link string, game *games.View) (string, Keyboard) {
	var b strings.Builder
	fmt.Fprintf(&b, "👥 %s\nУчастников: %d", card.Name, card.MemberCount)
	rows := make([][]Button, 0, 4)
	if card.Role == "OWNER" {
		b.WriteString("\nТы: владелец")
		rows = append(rows, []Button{{Text: "Участники", Data: fmt.Sprintf("grp:members:%d", card.ID)}})
	} else if card.OwnerName != "" {
		fmt.Fprintf(&b, "\nВладелец: %s", card.OwnerName)
	}
	if game != nil {
		fmt.Fprintf(&b, "\nТекущая игра: %s", game.Name)
		rows = append(rows, []Button{{Text: "Текущая игра", Data: fmt.Sprintf("game:open:%d", game.ID)}})
	} else if card.Role == "OWNER" || card.Role == "ADMIN" {
		rows = append(rows, []Button{{Text: "Создать игру", Data: fmt.Sprintf("game:create:%d", card.ID)}})
	}
	if link != "" {
		fmt.Fprintf(&b, "\n\nСсылка для приглашения:\n%s", link)
		rows = append(rows, []Button{{Text: "Пригласить", Copy: link}})
	}
	rows = append(rows, []Button{{Text: "Мои группы", Data: "grp:list"}})
	return b.String(), Keyboard{Rows: rows}
}

func membersScreen(card groups.Card, people []groups.Person) (string, Keyboard) {
	back := Keyboard{Rows: [][]Button{
		{{Text: "К группе", Data: fmt.Sprintf("grp:open:%d", card.ID)}},
	}}
	if card.Role != "OWNER" {
		text := fmt.Sprintf("👥 %s\nВладелец: %s\nУчастников: %d", card.Name, card.OwnerName, card.MemberCount)
		return text, back
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Участники «%s»:", card.Name)
	for _, person := range people {
		fmt.Fprintf(&b, "\n• %s — %s", person.DisplayName, roleLabel(person.Role))
	}
	return b.String(), back
}

func joinPreviewScreen(preview groups.Preview) (string, Keyboard) {
	text := fmt.Sprintf("🎄 Группа «%s»\nНажми «Вступить», чтобы присоединиться.", preview.GroupName)
	return text, Keyboard{Rows: [][]Button{
		{{Text: "Вступить", Data: fmt.Sprintf("inv:accept:%d", preview.InviteID)}},
		{{Text: "В меню", Data: "menu:home"}},
	}}
}

func joinedScreen(result groups.Join) (string, Keyboard) {
	var text string
	if result.AlreadyMember {
		text = fmt.Sprintf("Ты уже состоишь в группе «%s».\nУчастников: %d", result.GroupName, result.MemberCount)
	} else {
		text = fmt.Sprintf("🎄 Ты присоединился к группе «%s»!\nУчастников: %d", result.GroupName, result.MemberCount)
		if result.GameName != "" {
			text += fmt.Sprintf("\nТы автоматически зарегистрирован в игре «%s».", result.GameName)
		}
	}
	rows := [][]Button{
		{{Text: "Посмотреть участников", Data: fmt.Sprintf("grp:members:%d", result.GroupID)}},
	}
	if result.GameID != 0 {
		rows = append(rows, []Button{
			{Text: "Текущая игра", Data: fmt.Sprintf("game:open:%d", result.GameID)},
			{Text: "Отказаться", Data: fmt.Sprintf("game:decline:%d", result.GameID)},
		})
	}
	return text, Keyboard{Rows: rows}
}

func roleLabel(role string) string {
	switch role {
	case "OWNER":
		return "владелец"
	case "ADMIN":
		return "администратор"
	default:
		return "участник"
	}
}

func buttonLabel(name string) string {
	if utf8.RuneCountInString(name) <= 64 {
		return name
	}
	runes := []rune(name)
	return string(runes[:63]) + "…"
}

func deeplink(botUsername, token string) string {
	if botUsername == "" || token == "" {
		return ""
	}
	return "https://t.me/" + botUsername + "?start=join_" + token
}
