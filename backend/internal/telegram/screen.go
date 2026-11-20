package telegram

// Button is one inline keyboard button. Data is the callback payload.
type Button struct {
	Text string
	Data string
}

// Keyboard is an inline keyboard, one slice per row.
type Keyboard struct {
	Rows [][]Button
}

func mainKeyboard() Keyboard {
	return Keyboard{Rows: [][]Button{
		{{Text: "Мои группы", Data: "grp:list"}},
		{{Text: "Создать группу", Data: "grp:new"}},
		{{Text: "Помощь", Data: "menu:help"}},
	}}
}

const menuText = "🎄 Secret Santa"

const helpText = "🎄 Secret Santa\n\n" +
	"Бот ведёт Тайного Санту в личном чате. Группы здесь свои, это не чаты Telegram.\n\n" +
	"Мои группы — список твоих групп.\n" +
	"Создать группу — новая группа.\n" +
	"Помощь — это сообщение."

const noGroupsText = "У тебя пока нет групп."

const createLaterText = "Создание группы будет следующим шагом."

const invalidLinkText = "🎄 Secret Santa\n\nСсылка недействительна."

const unavailableText = "Это действие больше недоступно."
