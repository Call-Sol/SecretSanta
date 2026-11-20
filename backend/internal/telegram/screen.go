package telegram

// Button is one inline keyboard button. Data is the callback payload.
type Button struct {
	Text string
	Data string
	Copy string
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

const askGroupNameText = "Как назовём группу? Можно до 80 символов.\nОтмена — чтобы вернуться в меню."

const invalidNameText = "Нужно название от 1 до 80 символов."

const invalidLinkText = "🎄 Secret Santa\n\nСсылка недействительна."

const inviteInvalidText = "Ссылка недействительна или отозвана."

const notMemberText = "Ты не состоишь в этой группе."

const unavailableText = "Это действие больше недоступно."

const createFailedText = "Не получилось создать группу. Попробуй ещё раз."

const askGameNameText = "Как назовём игру? Можно до 80 символов.\nОтмена — чтобы вернуться в меню."

const askGameBudgetText = "Бюджет в рублях? Напиши число, или «-», если без бюджета.\nОтмена — чтобы вернуться в меню."

const askGameDeadlineText = "До какой даты идёт регистрация? Формат ДД.ММ.ГГГГ, или «-» без срока.\nОтмена — чтобы вернуться в меню."

const askGameExchangeText = "Дата обмена подарками? Формат ДД.ММ.ГГГГ, или «-».\nОтмена — чтобы вернуться в меню."

const invalidBudgetText = "Нужно целое число рублей или «-»."

const invalidDateText = "Нужна дата в формате ДД.ММ.ГГГГ или «-»."

const pastDateText = "Эта дата уже прошла. Укажи сегодня или позже, или «-»."

const forbiddenText = "Недостаточно прав."

const currentGameExistsText = "В группе уже есть незавершённая игра."

const gameFailedText = "Не получилось создать игру. Попробуй ещё раз."

const registrationClosedText = "Регистрация уже закрыта."

const notInGameText = "Ты не участвуешь в этой игре."
