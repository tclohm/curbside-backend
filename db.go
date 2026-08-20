package main

import (
	"database/sql"
	"errors"
	"modernc.org/sqlite"
)

// Sqlite c-level result code for constraint violation
const sqliteConstraintCode = 19

// reports whether err represents the client sending
// data that violates a table constraint 
func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error 
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqliteConstraintCode
}

// openDB opens (creating if necessary) the SQLite database at path 
// and makes sure the schema exists. Unlike main(), it returns an error 
// calling log.Fatal - that's what makes it callable from test later,
// with a real assertion on the error instead of the whole test process dying
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// sqlite only allows one writer at a time 
	// pragma settings are per-connection
	// sidesteps SQLite's "database is locked" error under 
	// concurrent writes
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return nil, err
	}

	_, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS reports (
			id TEXT PRIMARY KEY,
			plate TEXT NOT NULL,
			color TEXT NOT NULL,
			make_model TEXT,
			address TEXT NOT NULL,
			issue_type TEXT NOT NULL CHECK (issue_type IN (
				'Appears abandoned',
				'Blocking driveway',
				'Blocking hydrant',
				'Visibly disabled',
				'Other'
			)),
			note TEXT,
			status TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN (
				'submitted', 'under_review', 'resolved', 'withdrawn'
			)),
			corroboration_count INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS corroboration_responses (
			id TEXT PRIMARY KEY,
			report_id TEXT NOT NULL REFERENCES reports(id),
			nonce TEXT NOT NULL,
			answer TEXT NOT NULL CHECK (answer in ('still-there', 'gone', 'not-sure')),
			responded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (report_id, nonce)
		)
	`)
	if err != nil {
		return nil, err
	}

	return db, nil
}
