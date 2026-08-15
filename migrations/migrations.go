package migrations

import "embed"

// Files contains every SQL migration shipped with the Lapin binary.
//
//go:embed *.sql
var Files embed.FS
