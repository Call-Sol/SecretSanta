package telegram

import (
	"strings"
	"testing"

	"github.com/alekseymakarkin/secret-santa/internal/games"
	"github.com/alekseymakarkin/secret-santa/internal/groups"
)

func TestInviteButtonCopiesLink(t *testing.T) {
	link := "https://t.me/secret_santa_bot?start=join_abc"
	_, keyboard := groupCardScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "OWNER", MemberCount: 2,
	}, link, nil)
	var invite Button
	var found bool
	var create bool
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Text == "Пригласить" {
				invite = button
				found = true
			}
			if button.Text == "Создать игру" && button.Data == "game:create:1" {
				create = true
			}
			if button.Text == "Участники" && button.Data != "grp:members:1" {
				t.Fatalf("members button %+v", button)
			}
		}
	}
	if !found || invite.Copy != link || invite.Data != "" || !create {
		t.Fatalf("invite %+v create %v", invite, create)
	}
}

func TestGroupCardShowsTheCurrentGame(t *testing.T) {
	text, keyboard := groupCardScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "OWNER", MemberCount: 2,
	}, "", &games.View{ID: 9, Name: "Тайный Санта 2026"})
	if !strings.Contains(text, "Текущая игра: Тайный Санта 2026") {
		t.Fatalf("text %q", text)
	}
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Data == "game:create:1" {
				t.Fatal("create is offered while a game exists")
			}
			if button.Text == "Текущая игра" && button.Data != "game:open:9" {
				t.Fatalf("game button %+v", button)
			}
		}
	}
}

func TestMemberCardHidesTheRoster(t *testing.T) {
	text, keyboard := groupCardScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "MEMBER", OwnerName: "Анна", MemberCount: 4,
	}, "https://t.me/bot?start=join_abc", nil)
	if !strings.Contains(text, "Владелец: Анна") || !strings.Contains(text, "Участников: 4") {
		t.Fatalf("text %q", text)
	}
	if strings.Contains(text, "Борис") {
		t.Fatalf("roster leaked: %q", text)
	}
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Data == "grp:members:1" || button.Data == "game:create:1" {
				t.Fatal("member can open the roster or create a game")
			}
		}
	}
	limited, _ := membersScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "MEMBER", OwnerName: "Анна", MemberCount: 4,
	}, []groups.Person{{DisplayName: "Борис", Role: "MEMBER"}, {DisplayName: "Анна", Role: "OWNER"}})
	if strings.Contains(limited, "Борис") || !strings.Contains(limited, "Анна") || !strings.Contains(limited, "Участников: 4") {
		t.Fatalf("limited %q", limited)
	}
}
