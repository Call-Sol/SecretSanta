package telegram

import (
	"strings"
	"testing"

	"github.com/alekseymakarkin/secret-santa/internal/groups"
)

func TestInviteButtonCopiesLink(t *testing.T) {
	link := "https://t.me/secret_santa_bot?start=join_abc"
	_, keyboard := groupCardScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "OWNER", MemberCount: 2,
	}, link)
	var invite Button
	var found bool
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Text == "Пригласить" {
				invite = button
				found = true
			}
			if button.Text == "Участники" && button.Data != "grp:members:1" {
				t.Fatalf("members button %+v", button)
			}
		}
	}
	if !found || invite.Copy != link || invite.Data != "" {
		t.Fatalf("invite button %+v", invite)
	}
}

func TestMemberCardHidesTheRoster(t *testing.T) {
	text, keyboard := groupCardScreen(groups.Card{
		ID: 1, Name: "Друзья", Role: "MEMBER", OwnerName: "Анна", MemberCount: 4,
	}, "https://t.me/bot?start=join_abc")
	if !strings.Contains(text, "Владелец: Анна") || !strings.Contains(text, "Участников: 4") {
		t.Fatalf("text %q", text)
	}
	if strings.Contains(text, "Борис") {
		t.Fatalf("roster leaked: %q", text)
	}
	for _, row := range keyboard.Rows {
		for _, button := range row {
			if button.Data == "grp:members:1" {
				t.Fatal("member can open the roster")
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
