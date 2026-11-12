// Package myfi exposes embedded file systems for migrations and the compiled
// React frontend. The //go:embed directives must live in a file whose directory
// is an ancestor of the embedded paths — hence this root-level file.
package myfi

import "embed"

// MigrationsFS contains all SQL migration files.
//
//go:embed migrations
var MigrationsFS embed.FS

// WebFS contains the compiled React SPA (web/dist).
//
//go:embed web/dist
var WebFS embed.FS
