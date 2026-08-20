package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID  							 string `json:"id"`
	Plate							 string `json:"plate"`
	Color 	 					 string `json:"color"`
	MakeModel 				 string `json:"make_model,omitempty"`
	Address 					 string `json:"address"`
	IssueType 				 string `json:"issue_type"`
	Note 							 string `json:"node,omitempty"`
	Status 						 string `json:"status"`
	CorroborationCount int `json:"corroboration_count"`
	CreatedAt 				 string `json:"created_at"`
}

// createReportHandler takes dependency (db) our handler needs,
// and returns http.HandlerFunc that closes over it 
func createReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Report 
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if input. Plate == "" {
			http.Error(w, "plate is required", http.StatusBadRequest)
			return
		}
		
		// id, status, corroboration_count and created at are all ours to assign
		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		input.ID = id.String()
		input.Status = "submitted"
		input.CorroborationCount = 1
		input.CreatedAt = time.Now().UTC().Format(time.RFC3339)

		_, err = db.Exec(
			`INSERT INTO reports (id, plate, color, make_model, address, issue_type, notes,
		status, corroboration_count, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			 input.ID, input.Plate, input.Color, input.MakeModel, input.Address, input.IssueType,
			 input.Note, input.Status, input.CorroborationCount, input.CreatedAt,
		)
		if err != nil {
			if isConstraintError(err) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(input)
	}
}

// return every report, newest first 
func listReportsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, plate, color, make_model, address, issue_type, note, status, corroboration_count, created_at FROM reports ORDER BY created_at DESC
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
				&rep.Address, &rep.IssueType, &note, 
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
			SELECT id, plate, color, make_model, address, issue_type, note, status, corroboration_count, created_at FROM reports WHERE id = ?
		`, id).Scan(&rep.ID, &rep.Plate, &rep.Color, &makeModel, &rep.Address, &rep.IssueType, &note, &rep.Status, &rep.CorroborationCount, &rep.CreatedAt)
		
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
