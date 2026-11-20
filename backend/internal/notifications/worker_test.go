package notifications

import (
	"errors"
	"testing"
	"time"

	"github.com/alekseymakarkin/secret-santa/internal/telegram"
)

func TestClassifyBlocksAndRetries(t *testing.T) {
	failed, _, class := classify(&telegram.APIError{Status: 403, Description: "bot was blocked by the user"}, 1)
	if !failed || class != "blocked" {
		t.Fatalf("blocked %v %s", failed, class)
	}
	failed, wait, class := classify(&telegram.APIError{Status: 429, RetryAfter: 7 * time.Second}, 1)
	if failed || wait != 7*time.Second || class != "rate limited" {
		t.Fatalf("retry %v %s %s", failed, wait, class)
	}
	failed, wait, class = classify(errors.New("timeout"), 2)
	if failed || wait != 15*time.Second || class != "telegram unavailable" {
		t.Fatalf("network %v %s %s", failed, wait, class)
	}
	failed, _, class = classify(errors.New("timeout"), 8)
	if !failed || class != "too many attempts" {
		t.Fatalf("give up %v %s", failed, class)
	}
}
