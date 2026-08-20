package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID  			string `json:"id"`
	Plate			string `json:"plate"`
	Color 	 	string `json:"color"`
	MakeModel string `json:"make_model,omitempty"`
	Address 	string `json:"address"`
	IssueType string `json:"issue_type"`
	Note 			string `json:"node,omitempty"`
	CreatedAt string `json:"created_at"`
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

		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		input.ID = id.String()
		input.CreatedAt = time.Now().UTC().Format(time.RFC3339)

		_, err = db.Exec(
			`INSERT INTO reports (id, plate, color, make_model, address, issue_type, notes,
		created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			 input.ID, input.Plate, input.Color, input.MakeModel, input.Addresss, input.IssueType,
			 input.Note, input.CreatedAt,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(input)
	}
}
