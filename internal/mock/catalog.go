package mock

import (
	"fmt"
	"strings"
)

// MatchCatalogQuery intercepts standard PostgreSQL catalog and ORM introspection queries.
// This allows Prisma, Drizzle, GORM, TypeORM, and Hibernate to connect and introspect without errors.
func MatchCatalogQuery(query string) *Rule {
	norm := strings.ToUpper(NormalizeSQL(query))

	// 1. Current schema
	if strings.Contains(norm, "CURRENT_SCHEMA()") || strings.Contains(norm, "CURRENT_SCHEMA") {
		return &Rule{
			ID:      "builtin-current-schema",
			Query:   query,
			Columns: []string{"current_schema"},
			Types:   []int32{25}, // text
			Rows:    [][]string{{"public"}},
			Tag:     "SELECT 1",
		}
	}

	// 2. Version
	if strings.Contains(norm, "VERSION()") {
		return &Rule{
			ID:      "builtin-version",
			Query:   query,
			Columns: []string{"version"},
			Types:   []int32{25},
			Rows:    [][]string{{"PostgreSQL 16.2 (PGWire-Mock) on x86_64-pc-linux-gnu, compiled by gcc, 64-bit"}},
			Tag:     "SELECT 1",
		}
	}

	// 3. Recovery status
	if strings.Contains(norm, "PG_IS_IN_RECOVERY()") {
		return &Rule{
			ID:      "builtin-recovery",
			Query:   query,
			Columns: []string{"pg_is_in_recovery"},
			Types:   []int32{16}, // bool
			Rows:    [][]string{{"f"}},
			Tag:     "SELECT 1",
		}
	}

	// 4. Current transaction ID
	if strings.Contains(norm, "TXID_CURRENT()") || strings.Contains(norm, "PG_CURRENT_XACT_ID()") {
		return &Rule{
			ID:      "builtin-txid",
			Query:   query,
			Columns: []string{"txid_current"},
			Types:   []int32{20}, // int8
			Rows:    [][]string{{"1000"}},
			Tag:     "SELECT 1",
		}
	}

	// 5. pg_type queries (Prisma, pgx, lib/pq type loaders)
	if strings.Contains(norm, "FROM PG_TYPE") || strings.Contains(norm, "FROM PG_CATALOG.PG_TYPE") {
		return buildPGTypeCatalogRule(query)
	}

	// 6. pg_namespace queries
	if strings.Contains(norm, "FROM PG_NAMESPACE") || strings.Contains(norm, "FROM PG_CATALOG.PG_NAMESPACE") {
		return &Rule{
			ID:      "builtin-pg-namespace",
			Query:   query,
			Columns: []string{"oid", "nspname", "nspowner"},
			Types:   []int32{26, 25, 26}, // oid, text, oid
			Rows: [][]string{
				{"11", "pg_catalog", "10"},
				{"99", "pg_toast", "10"},
				{"2200", "public", "10"},
				{"11564", "information_schema", "10"},
			},
			Tag: "SELECT 4",
		}
	}

	// 7. pg_class queries
	if strings.Contains(norm, "FROM PG_CLASS") || strings.Contains(norm, "FROM PG_CATALOG.PG_CLASS") {
		return &Rule{
			ID:      "builtin-pg-class",
			Query:   query,
			Columns: []string{"oid", "relname", "relnamespace", "relkind", "reltuples"},
			Types:   []int32{26, 25, 26, 18, 700}, // oid, name, oid, char, float4
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	// 8. pg_attribute queries (table column inspection)
	if strings.Contains(norm, "FROM PG_ATTRIBUTE") || strings.Contains(norm, "FROM PG_CATALOG.PG_ATTRIBUTE") {
		return &Rule{
			ID:      "builtin-pg-attribute",
			Query:   query,
			Columns: []string{"attrelid", "attname", "atttypid", "attnum", "attnotnull"},
			Types:   []int32{26, 25, 26, 21, 16}, // oid, name, oid, int2, bool
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	// 9. pg_settings
	if strings.Contains(norm, "FROM PG_SETTINGS") || strings.Contains(norm, "FROM PG_CATALOG.PG_SETTINGS") {
		return &Rule{
			ID:      "builtin-pg-settings",
			Query:   query,
			Columns: []string{"name", "setting", "unit", "category", "short_desc"},
			Types:   []int32{25, 25, 25, 25, 25},
			Rows: [][]string{
				{"server_version", "16.2 (PGWire-Mock)", "", "Compatibility", "Server version"},
				{"standard_conforming_strings", "on", "", "Compatibility", "Causes '...' to treat backslashes literally"},
				{"client_encoding", "UTF8", "", "Client Connection Defaults", "Sets the client's character set encoding"},
				{"DateStyle", "ISO, MDY", "", "Client Connection Defaults", "Sets the display format for date and time values"},
				{"TimeZone", "UTC", "", "Client Connection Defaults", "Sets the time zone for displaying and interpreting time stamps"},
			},
			Tag: "SELECT 5",
		}
	}

	// 10. information_schema.tables
	if strings.Contains(norm, "INFORMATION_SCHEMA.TABLES") {
		return &Rule{
			ID:      "builtin-info-schema-tables",
			Query:   query,
			Columns: []string{"table_catalog", "table_schema", "table_name", "table_type"},
			Types:   []int32{25, 25, 25, 25},
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	// 11. information_schema.columns
	if strings.Contains(norm, "INFORMATION_SCHEMA.COLUMNS") {
		return &Rule{
			ID:      "builtin-info-schema-columns",
			Query:   query,
			Columns: []string{"table_catalog", "table_schema", "table_name", "column_name", "ordinal_position", "is_nullable", "data_type"},
			Types:   []int32{25, 25, 25, 25, 23, 25, 25},
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	// 12. information_schema.schemata
	if strings.Contains(norm, "INFORMATION_SCHEMA.SCHEMATA") {
		return &Rule{
			ID:      "builtin-info-schema-schemata",
			Query:   query,
			Columns: []string{"catalog_name", "schema_name", "schema_owner"},
			Types:   []int32{25, 25, 25},
			Rows: [][]string{
				{"postgres", "public", "postgres"},
				{"postgres", "information_schema", "postgres"},
				{"postgres", "pg_catalog", "postgres"},
			},
			Tag: "SELECT 3",
		}
	}

	// 13. pg_range
	if strings.Contains(norm, "FROM PG_RANGE") || strings.Contains(norm, "FROM PG_CATALOG.PG_RANGE") {
		return &Rule{
			ID:      "builtin-pg-range",
			Query:   query,
			Columns: []string{"rngtypid", "rngsubtype"},
			Types:   []int32{26, 26},
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	// 14. pg_enum
	if strings.Contains(norm, "FROM PG_ENUM") || strings.Contains(norm, "FROM PG_CATALOG.PG_ENUM") {
		return &Rule{
			ID:      "builtin-pg-enum",
			Query:   query,
			Columns: []string{"enumtypid", "enumsortorder", "enumlabel"},
			Types:   []int32{26, 700, 25},
			Rows:    [][]string{},
			Tag:     "SELECT 0",
		}
	}

	return nil
}

func buildPGTypeCatalogRule(query string) *Rule {
	cols := []string{"oid", "typname", "typnamespace", "typlen", "typtype", "typcategory", "typelem", "typarray"}
	types := []int32{26, 25, 26, 21, 18, 18, 26, 26}

	// Common standard type metadata
	typeDefs := [][]string{
		{"16", "bool", "11", "1", "b", "B", "0", "1000"},
		{"17", "bytea", "11", "-1", "b", "U", "0", "1001"},
		{"18", "char", "11", "1", "b", "S", "0", "1002"},
		{"19", "name", "11", "64", "b", "S", "18", "1003"},
		{"20", "int8", "11", "8", "b", "N", "0", "1016"},
		{"21", "int2", "11", "2", "b", "N", "0", "1005"},
		{"23", "int4", "11", "4", "b", "N", "0", "1007"},
		{"25", "text", "11", "-1", "b", "S", "0", "1009"},
		{"26", "oid", "11", "4", "b", "N", "0", "1028"},
		{"114", "json", "11", "-1", "b", "U", "0", "199"},
		{"700", "float4", "11", "4", "b", "N", "0", "1021"},
		{"701", "float8", "11", "8", "b", "N", "0", "1022"},
		{"1043", "varchar", "11", "-1", "b", "S", "0", "1015"},
		{"1082", "date", "11", "4", "b", "D", "0", "1182"},
		{"1083", "time", "11", "8", "b", "D", "0", "1183"},
		{"1114", "timestamp", "11", "8", "b", "D", "0", "1115"},
		{"1184", "timestamptz", "11", "8", "b", "D", "0", "1185"},
		{"1700", "numeric", "11", "-1", "b", "N", "0", "1231"},
		{"2950", "uuid", "11", "16", "b", "U", "0", "2951"},
		{"3802", "jsonb", "11", "-1", "b", "U", "0", "3807"},
	}

	return &Rule{
		ID:      "builtin-pg-type",
		Query:   query,
		Columns: cols,
		Types:   types,
		Rows:    typeDefs,
		Tag:     fmt.Sprintf("SELECT %d", len(typeDefs)),
	}
}
