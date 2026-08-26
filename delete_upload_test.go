package main

import (
	"net/http"
	"os"
	"testing"
)

func TestDeletePendingUpload_Success(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)

	// use a real file on disk so we can confirm 
	// it actually gets removed not just the DB row 
	photoPath := "testdata/delete_me.jpg"
	if err := os.WriteFile(photoPath, []byte("fake jpeg bytes"), 0644); err != nil {
		t.Fatalf("writing test fixture: %v", err)
	}
	t.Cleanup(func() { os.Remove(photoPath) }) // in case the test fails before deletion 

	// insert directly, reusing the same shape as insertTestPendingUpload 
	// but with our own photoPath so we can verify the file disappears 
	pendingID := insertTestPendingUploadWithPhoto(t, db, photoPath)

	rec := doRequest(t, mux, "DELETE", "/photos/"+pendingID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if _, err := os.Stat(photoPath); !os.IsNotExist(err) {
		t.Errorf("expected photo file to be deleted, but it still exists (or a different error occurred: %v)", err)
	}

	var count int 
	if err := db.QueryRow(`SELECT COUNT(*) FROM pending_uploads WHERE id = ?`, pendingID).Scan(&count); err != nil {
		t.Fatalf("query pending_uploads: %v", err)
	}

	if count != 0 {
		t.Errorf("pending_uploads row still exists after delete")
	}
}

func TestDeletePendingUpload_NonExistentIsStillNoContent(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)
	rec := doRequest(t, mux, "DELETE", "/photos/not-a-real-id", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (deleting something already gone is idempotent, not an error)", rec.Code, http.StatusNoContent)
	}
}
