package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rwcarlsen/goexif/exif"
)

// cap the whole incoming request body for POST /photos 
// mobile app compresses / converts photos to JPEG before sending 
// target -200KB - 1MB 
// 5MB lease real headroom
const maxUploadBytes = 5 << 20 // 5MB 

type pendingUploadResponse struct {
	ID 		 		string `json:"id"`
	ExpiresAt string `json:"expires_at"`
}

// accepts a photo + device lat/lng, validates it, saves it to disk, 
// and create a pending_uploads row the client will later reference from 
// POST /reports (or cancel vial DELETE /photos/{id})
func createPendingUploadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "request too larget or malformed", http.StatusBadRequest)
			return
		}
		
		file, _, err := r.FormFile("photo")
		if err != nil {
			http.Error(w, "photo is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		imgBytes, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "could not read photo", http.StatusInternalServerError)
			return
		}

		if !isJPEG(imgBytes) {
			http.Error(w, "photo must be a JPEG", http.StatusBadRequest)
			return
		}

		lat, err := strconv.ParseFloat(r.FormValue("lat"), 64)
		if err != nil {
			http.Error(w, "lat is required and must be a number", http.StatusBadRequest)
			return
		}

		lng, err := strconv.ParseFloat(r.FormValue("lng"), 64)
		if err != nil {
			http.Error(w, "lng is required and must be a number", http.StatusBadRequest)
			return
		}

		if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			http.Error(w, "lat/lng out of valid range", http.StatusBadRequest)
			return
		}

		exifData, err := exif.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			http.Error(w, "photo has no location data", http.StatusBadRequest)
			return
		}

		exifLat, exifLng, err := exifData.LatLong()
		if err != nil {
			http.Error(w, "photo has no location data", http.StatusBadRequest)
			return
		}


		distance := haversineDistanceMeters(lat, lng, exifLat, exifLng)
		if distance > gpsMismatchThresholdMeters {
			http.Error(w, "photo location does not match your location", http.StatusBadRequest)
			return
		}


		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		photoPath, err := savePendingPhoto(id.String(), imgBytes)
		if err != nil {
			http.Error(w, "could not save photo", http.StatusInternalServerError)
			return
		}

		createdAt := time.Now().UTC()
		_, err = db.Exec(
			`INSERT INTO pending_uploads (id, photo_path, lat, lng, exif_lat, exif_lng, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			 id.String(), photoPath, lat, lng, createdAt.Format(time.RFC3339),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := pendingUploadResponse{
			ID: 			 id.String(),
			ExpiresAt: createdAt.Add(pendingUploadTTL).Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// gps mismatch threshold meters is how far apart device-reported lat/lng 
// photos EXIF lat/lng are allowed to have before a submission is rejected
// 150m leaves real headroom for drift while still catching a geniunely diff location
const gpsMismatchThresholdMeters = 150.0

// earth mean radius
const earthRadiusMeters = 6371000.0

// returns the great-circle distance in meters btwn two lat/lng points 
func haversineDistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	
	lat1Rad, lat2Rad := toRad(lat1), toRad(lat2)
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)

	a := math.Sin(dLat / 2) * math.Sin(dLat / 2) + 
			 math.Cos(lat1Rad) * math.Cos(lat2Rad) * math.Sin(dLng / 2) * math.Sin(dLng / 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1 - a))

	return earthRadiusMeters * c
}


// photos live while a report is still being composed (photo uploaded, ML pending/done, user has not hit final submut yet) Flat - not nested by date - because rows here live at most 
// pendingUploadTTL (8 minutes) before the sweeper deletes them
const pendingUploadsDir = "uploads/pending"

// write photo bytes to disk under pendingUploadDir, named by the pending_uploads id 
// and returns the path to store in that row's photo_path column
func savePendingPhoto(id string, data []byte) (string, error) {
	path := pendingUploadsDir + "/" + id + ".jpg"
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

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
		if err := deletePendingUpload(db, e.id, e.photoPath); err != nil {
			log.Printf("[ERROR] sweeping pending_upload %s: %v", e.id, err)
		}
	}

	return nil
}

// removes pending_uploads row's photo file from disk first, 
// then deletes the DB row - same ordering reasoning as the 
// sweep above: a row left behind by a failure still points at a photo
// that's either still there (safe: retryable) or already gone (safe: 
// next attempt's file delete is a harmless no-op)
func deletePendingUpload(db *sql.DB, id, photoPath string) error {
	if err := os.Remove(photoPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, err := db.Exec(`DELETE FROM pending_uploads WHERE id = ?`, id)
	return err
}

// handles an explicit user cancel: delete the photo and pending_uploads
// row immediately, rather than waitiung for the sweeper 
// Idemponent - deleting an id that's already gone (expired, already submitted,
// or never existed) is still a 204: the end state the caller wants ("this doesnt exist")
// is already true either way
func deletePendingUploadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		
		var photoPath string
		err := db.QueryRow(`SELECT photo_path FROM pending_uploads WHERE id = ?`, id).Scan(&photoPath)
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := deletePendingUpload(db, id, photoPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
