package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fresh, empty, in-memory SQLite
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// builds a request, routes it through
// the real mux and returns the recorded response
func doRequest(t *testing.T, mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// pending_uploads row directy into the test DB 
// exercising the photo/EXIF pipeline, just report creation
// and returns its id, ready to use as pending_uploads_id in 
// a POST /reports payload
func insertTestPendingUpload(t *testing.T, db *sql.DB) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO pending_uploads (id, photo_path, lat, lng, exif_lat, exif_lng, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), "testdata/fake.jpg", 33.7952, -118.3088, 33.7952, -118.3088, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("inserting test pending_upload: %v", err)
	}
	return id.String()
}
