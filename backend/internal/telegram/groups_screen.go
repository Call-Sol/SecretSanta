package telegram

import (
	"fmt"
	"strings"
	"unicode/utf8"

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

func groupCardScreen(card groups.Card, link string) (string, Keyboard) {
	var b strings.Builder
	fmt.Fprintf(&b, "👥 %s\nУчастников: %d\nТы: %s", card.Name, card.MemberCount, roleLabel(card.Role))
	if link != "" {
		fmt.Fprintf(&b, "\n\nСсылка для приглашения:\n%s", link)
	}
	return b.String(), Keyboard{Rows: [][]Button{
		{{Text: "Участники", Data: fmt.Sprintf("grp:members:%d", card.ID)}},
		{{Text: "Пригласить", Data: fmt.Sprintf("grp:open:%d", card.ID)}},
		{{Text: "Мои группы", Data: "grp:list"}},
	}}
}

func membersScreen(groupID int64, name string, people []groups.Person) (string, Keyboard) {
	var b strings.Builder
	fmt.Fprintf(&b, "Участники «%s»:", name)
	for _, person := range people {
		fmt.Fprintf(&b, "\n• %s — %s", person.DisplayName, roleLabel(person.Role))
	}
	return b.String(), Keyboard{Rows: [][]Button{
		{{Text: "К группе", Data: fmt.Sprintf("grp:open:%d", groupID)}},
	}}
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
	}
	return text, Keyboard{Rows: [][]Button{
		{{Text: "Посмотреть участников", Data: fmt.Sprintf("grp:members:%d", result.GroupID)}},
	}}
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
