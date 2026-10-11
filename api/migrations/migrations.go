package migrations

import _ "embed"

// InitSQL contains the schema from 001_init.sql.
//
//go:embed 001_init.sql
var InitSQL string
