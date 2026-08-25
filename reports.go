package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID  							 string `json:"id"`
	Plate							 string `json:"plate"`
	Color 	 					 string `json:"color"`
	MakeModel 				 string `json:"make_model,omitempty"`
	Address 					 string `json:"address"`
	PhotoPath 				 string `json:"photo_path"`
	IssueType 				 string `json:"issue_type"`
	Note							 string `json:"note,omitempty"`
	Status 						 string `json:"status"`
	CorroborationCount int `json:"corroboration_count"`
	CreatedAt 				 string `json:"created_at"`
}

// references a pending_uploads row
type createReportInput struct {
	Plate           string `json:"plate"`
	Color           string `json:"color"`
	MakeModel       string `json:"make_model"`
	Address         string `json:"address"`
	IssueType       string `json:"issue_type"`
	Note            string `json:"note"`
	PendingUploadID string `json:"pending_upload_id"`
}

// createReportHandler takes dependency (db) our handler needs,
// and returns http.HandlerFunc that closes over it 
func createReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createReportInput 
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if input.Plate == "" {
			http.Error(w, "plate is required", http.StatusBadRequest)
			return
		}

		if input.PendingUploadID == "" {
			http.Error(w, "pending_upload_id is required", http.StatusBadRequest)
			return
		}

		// pending upload server-side rather than trusting
		// client-supplied photo path directly
		var pendingPhotoPath, pendingCreatedAt string 
		err := db.QueryRow(
			`SELECT photo_path, created_at FROM pending_uploads WHERE id = ?`,
			input.PendingUploadID,
		).Scan(&pendingPhotoPath, &pendingCreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "pending upload not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		createdAt, err := time.Parse(time.RFC3339, pendingCreatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// auth expiry check - independent of whether the background sweeper
		// has gotten to this row yet
		if time.Since(createdAt) > pendingUploadTTL {
			http.Error(w, "pending upload has expired", http.StatusGone)
			return
		}

		// id, status, corroboration_count and created at are all ours to assign
		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		report := Report{
			ID: 								id.String(),
			Plate: 							input.Plate,
			Color: 							input.Color,
			MakeModel: 					input.MakeModel,
			Address: 						input.Address,
			PhotoPath:					reportPhotoPath(id.String(), time.Now().UTC()),
			IssueType: 					input.IssueType,
			Note: 							input.Note,
			Status: 						"submitted",
			CorroborationCount: 1,
			CreatedAt: 					time.Now().UTC().Format(time.RFC3339),
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, err = tx.Exec(
			`INSERT INTO reports (id, plate, color, make_model, address, photo_path, issue_type, note,
		status, corroboration_count, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			 report.ID, report.Plate, report.Color, report.MakeModel, report.Address, report.PhotoPath, report.IssueType,
			 report.Note, report.Status, report.CorroborationCount, report.CreatedAt,
		)
		if err != nil {
			if isConstraintError(err) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := tx.Exec(`DELETE FROM pending_uploads WHERE id = ?`, input.PendingUploadID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// DB is now the source of truth: the report exists, and the pending upload is gone 
		// move the actual file to match 
		// if this fails, the report still exists correctly -- its photo_path just wont resolve to a file yet
		// accepted gap rather than something silently swallowed
		if err := os.MkdirAll(filepath.Dir(report.PhotoPath), 0755); err != nil {
			log.Printf("[ERROR] - could not create report photo directory for %s: %v", report.ID, err)
		} else if err := os.Rename(pendingPhotoPath, report.PhotoPath); err != nil {
			log.Printf("[ERRPR] - could not move photo for report %s: %v", report.ID, err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(report)
	}
}

// report photo path build the permanent
// photo : uploads/reports/{yyyy}/{mm}/{dd}/{id}.jpg 
//	-- long-lived and what makes future retention / cleanup (resolved 3+ months ago)
//  tractable ago
func reportPhotoPath(id string, t time.Time) string {
	return filepath.Join("uploads", "reports", t.Format("2006"), t.Format("01"), t.Format("02"), id+".jpg")
}

// return every report, newest first 
func listReportsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, plate, color, make_model, address, photo_path, issue_type, note, status, corroboration_count, created_at FROM reports ORDER BY created_at DESC
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		reports := []Report{}

		for rows.Next() {
			var rep Report 
			var makeModel, note sql.NullString
			if err := rows.Scan(
				&rep.ID, &rep.Plate, &rep.Color, &makeModel,
				&rep.Address, &rep.PhotoPath, &rep.IssueType, &note, 
				&rep.Status, &rep.CorroborationCount, &rep.CreatedAt,
			); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rep.MakeModel = makeModel.String 
			rep.Note = note.String 
			reports = append(reports, rep)
		}

		// rows.Next() returns false when Query itself failed partway
		if err := rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} 

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reports)
	}
}

func getReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var rep Report 
		var makeModel, note sql.NullString
		err := db.QueryRow(`
			SELECT id, plate, color, make_model, address, photo_path, 
			issue_type, note, status, corroboration_count, created_at 
			FROM reports WHERE id = ?
		`, id).Scan(&rep.ID, &rep.Plate, &rep.Color, &makeModel, 
		&rep.Address, &rep.PhotoPath, &rep.IssueType, &note, &rep.Status, 
		&rep.CorroborationCount, &rep.CreatedAt)
		
		// sql.ErrNoRows -- return when the query matched nothing - errors.Is check for that 
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "report not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rep.MakeModel = makeModel.String
		rep.Note = note.String

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rep)
	}
}
