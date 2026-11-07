// Package migrations embeds the SQL migrations. hg-server applies them on
// startup in file-name order (see internal/platform/db.Migrate).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
