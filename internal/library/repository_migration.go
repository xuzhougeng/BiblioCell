package library

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// SQLite cannot extend a CHECK constraint in place. Rebuild the repository
// table with foreign keys disabled so its dataset associations are not cascaded
// away. Keep IDs and every column, and check associations before committing.
// Inspect the actual DDL, like healColumns, rather than trusting user_version.
func migrateRepositoryNames(db *sql.DB) (err error) {
	var ddl string
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'repository_records'").Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // schema.sql creates a fresh table.
	}
	if err != nil {
		return fmt.Errorf("read repository schema: %w", err)
	}
	constraint := regexp.MustCompile(`repository IN\s*\([^)]*\)`)
	if !constraint.MatchString(ddl) {
		return nil
	}
	expanded := constraint.ReplaceAllString(ddl, "length(trim(repository)) BETWEEN 1 AND 100 AND repository = trim(repository)")
	expanded = strings.Replace(expanded, "repository_records", "repository_records_custom", 1)
	if _, err = db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("prepare repository migration: %w", err)
	}
	defer func() {
		_, restoreErr := db.Exec("PRAGMA foreign_keys = ON")
		err = errors.Join(err, restoreErr)
	}()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		expanded,
		"INSERT INTO repository_records_custom SELECT * FROM repository_records",
		"DROP TABLE repository_records",
		"ALTER TABLE repository_records_custom RENAME TO repository_records",
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migrate custom repositories: %w", err)
		}
	}
	rows, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	invalid := rows.Next()
	checkErr := rows.Err()
	closeErr := rows.Close()
	if invalid {
		return errors.New("repository migration contains foreign key violations")
	}
	if err := errors.Join(checkErr, closeErr); err != nil {
		return err
	}
	return tx.Commit()
}
