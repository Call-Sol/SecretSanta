package postgres

import (
	"strings"
	"testing"
)

func TestMigrationFilesIncludeSkeleton(t *testing.T) {
	names, err := migrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "00001_init.sql") {
			return
		}
	}
	t.Fatalf("skeleton migration missing: %v", names)
}
