package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"
)

// check the bytes of an uploaded file rather than trusting
// the client-supplied Content-Type header (easily wrong or spoofed)
// Sniff the first bytes against known file signature - for jpeg 
func isJPEG(data []byte) bool {
	return http.DetectContentType(data) == "image/jpeg"
}

// how long is pending_uploads row is allowed to live
// before it's considered expired. This is the same 8-minute window
// enforced authoritatively in POST /reports (check fresh agains created_at 
// submit time) -- sweep for clean up
const pendingUploadTTL = 8 * time.Minute

// launches a background goroutine that periodically 
// deletes expired pending_uploads row (and their photo files on disk).
// runs for the lifetime of the process; intended to be called once
func startExpirySweeper(db *sql.DB, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := sweepExpiredUploads(db); err != nil {
				log.Printf("[ERROR] expiry sweep failed: %v", err)
			}
		}
	}()
}

// finds pending_uploads row older than 
// pendingUploadTTL, deletes each ones photo file form disk first,
// then deletes the DB row. Deleting the file first means a row left behind 
// by a failure (process killed mid-sweep) still points at a photo that 
// either still exists (safe: retried next cycle) or is already gone 
// (safe: next cycle's file delete become a harmless no-op) - never a DB 
// row pointing at nothing with no way to notice
func sweepExpiredUploads(db *sql.DB) error {
	cutoff := time.Now().UTC().Add(-pendingUploadTTL).Format(time.RFC3339)

	rows, err := db.Query(
		`SELECT id, photo_path FROM pending_uploads WHERE created_at < ?`,
		cutoff,
	)
	if err != nil {
		return err
	}

	type expired struct {
		id 				string 
		photoPath string
	}

	var toDelete []expired 
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.id, &e.photoPath); err != nil {
			rows.Close()
			return err
		}
		toDelete = append(toDelete, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, e := range toDelete {
		// os.Remove returning "file doesnt exist" is fine here - that's 
		// the harmless-no-op case described above, not a real failure
		if err := os.Remove(e.photoPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[ERROR] could not remove photo %s for pending_upload %s: %v", e.photoPath, e.id, err)
			continue // leave the DB row for the next sweep to retry
		}

		if _, err := db.Exec(`DELETE FROM pending_uploads WHERE id = ?`, e.id); err != nil {
			log.Printf("[ERROR] could not delete pending_upload %s: %v", e.id, err)
		}
	}

	return nil
}
