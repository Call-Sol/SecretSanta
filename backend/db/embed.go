// Package db embeds SQL migrations.
//
// The embed directive has to live in this package: Go embed paths cannot
// contain "..", so internal/platform/postgres cannot reach these files.
package db

import "embed"

// Migrations is the goose SQL tree. Files are under the migrations/ prefix.
//
//go:embed migrations/*.sql
var Migrations embed.FS
