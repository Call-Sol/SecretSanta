package telegram

import (
	"strings"
	"testing"

	"github.com/alekseymakarkin/secret-santa/internal/games"
)

func TestGameCardListsParticipantsOnlyForOrganizers(t *testing.T) {
	_, owner := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "OWNER", Status: "REGISTRATION"}, "", "")
	_, member := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "MEMBER", Status: "REGISTRATION", Mine: games.Pending}, "", "")
	if !hasData(owner, "game:people:5") {
		t.Fatal("owner has no participant list")
	}
	if hasData(member, "game:people:5") {
		t.Fatal("member can open the participant list")
	}
}

func TestPeopleScreenKeepsWaitingOutOfConfirmed(t *testing.T) {
	text, _ := gamePeopleScreen(games.View{ID: 5, Name: "Тайный Санта 2026"}, []games.Participant{
		{DisplayName: "Анна", Status: games.Confirmed, AssignmentState: "ASSIGNED"},
		{DisplayName: "Виктор", Status: games.Confirmed, AssignmentState: "AWAITING_ASSIGNMENT"},
	})
	if !strings.Contains(text, "Подтвердили:\n• Анна") || !strings.Contains(text, "Ожидают включения:\n• Виктор") {
		t.Fatalf("text %q", text)
	}
	if strings.Contains(text, "Подтвердили:\n• Виктор") || !strings.Contains(text, "Ожидают решения:\n• нет") {
		t.Fatalf("text %q", text)
	}
}

func TestOrganizerCanStartTheDraw(t *testing.T) {
	_, keyboard := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "OWNER", Status: "REGISTRATION"}, "", "")
	if !hasData(keyboard, "game:draw:5") {
		t.Fatal("owner cannot start the draw")
	}
	_, member := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "MEMBER", Status: "REGISTRATION"}, "", "")
	if hasData(member, "game:draw:5") {
		t.Fatal("member can start the draw")
	}
	_, drawn := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "MEMBER", Status: "DRAWN", Assignment: "ASSIGNED"}, "", "")
	if hasData(drawn, "game:draw:5") || !hasData(drawn, "game:mine:5") {
		t.Fatal("drawn card buttons")
	}
}

func TestOrganizerSeesTheGameInvite(t *testing.T) {
	link := "https://t.me/secret_santa_bot?start=play_abc"
	text, keyboard := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "OWNER", Status: "REGISTRATION"}, "", link)
	if !strings.Contains(text, link) || !hasCopy(keyboard, link) {
		t.Fatalf("owner card %q buttons %+v", text, keyboard.Rows)
	}
	memberText, member := gameCardScreen(games.View{ID: 5, GroupID: 3, Role: "MEMBER", Status: "REGISTRATION"}, "", link)
	if strings.Contains(memberText, "play_abc") || hasCopy(member, link) {
		t.Fatal("member sees the game invite")
	}
}

func hasCopy(keyboard Keyboard, link string) bool {
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Copy == link {
				return true
			}
		}
	}
	return false
}

func hasData(keyboard Keyboard, data string) bool {
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Data == data {
				return true
			}
		}
	}
	return false
}
