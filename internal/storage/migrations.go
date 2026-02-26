package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/duckdb/*.sql
var duckdbMigrations embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

// RunMigrations executes any SQL migration files that have not yet been applied.
// It maintains a schema_migrations table in the target database to track which
// files have already run, so each migration executes exactly once.
func RunMigrations(db *sql.DB, migrations embed.FS, dir string) error {
	// Ensure the tracking table exists.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	// Load the set of already-applied migrations.
	rows, err := db.Query(`SELECT filename FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("querying schema_migrations: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		applied[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Read and sort migration files.
	entries, err := fs.ReadDir(migrations, dir)
	if err != nil {
		return fmt.Errorf("reading migration dir %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if applied[entry.Name()] {
			continue
		}

		data, err := fs.ReadFile(migrations, dir+"/"+entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("executing migration %s: %w", entry.Name(), err)
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (filename) VALUES (?)`, entry.Name()); err != nil {
			return fmt.Errorf("recording migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}
