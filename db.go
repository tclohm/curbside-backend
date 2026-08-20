package main

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// openDB opens (creating if necessary) the SQLite database at path 
// and makes sure the schema exists. Unlike main(), it returns an error 
// calling log.Fatal - that's what makes it callable from test later,
// with a real assertion on the error instead of the whole test process dying
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(
		`CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY,
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
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, err
	}

	return db, nil
}
