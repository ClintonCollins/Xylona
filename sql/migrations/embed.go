// Package migrations provides the embedded SQL migration files.
package migrations

import "embed"

// Root is the embedded migration filesystem root.
const Root = "."

// FS contains the embedded SQL migration files.
//
//go:embed *.sql
var FS embed.FS
