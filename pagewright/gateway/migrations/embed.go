// Package migrations contains the canonical gateway schema migrations.
package migrations

import "embed"

// Files is bundled into every gateway binary, so startup needs no SQL directory.
//
//go:embed *.up.sql
var Files embed.FS
