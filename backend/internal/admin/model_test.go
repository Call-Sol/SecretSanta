package admin

import (
	"testing"
	"time"
)

func TestNormalizeGroupName(t *testing.T) {
	name, err := NormalizeGroupName("  Friends  ")
	if err != nil || name != "Friends" {
		t.Fatalf("name %q err %v", name, err)
	}
	if _, err := NormalizeGroupName("   "); err != ErrInvalidName {
		t.Fatalf("got %v", err)
	}
}

func TestInviteDecorate(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	limit := 1
	inv := Invite{UsedCount: 1, MaxUses: &limit, ExpiresAt: &past}
	inv.decorate(now)
	if inv.State != "Expired" {
		t.Fatalf("state %s", inv.State)
	}

	revoked := now
	inv = Invite{RevokedAt: &revoked, ExpiresAt: &past}
	inv.decorate(now)
	if inv.State != "Revoked" {
		t.Fatalf("state %s", inv.State)
	}

	inv = Invite{UsedCount: 2}
	inv.decorate(now)
	if inv.State != "Active" || inv.Usage != "2 / unlimited" {
		t.Fatalf("state %s usage %s", inv.State, inv.Usage)
	}
}
