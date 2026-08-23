package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type CorroborationResponse struct {
	ID 			 		string `json:"id"`
	ReportID 		string `json:"report_id"`
	Nonce 			string `json:"nonce"`
	Answer 			string `json:"answer"`
	ResponsedAt	string `json:"Responsed_at"`
}

// look at prior response for this
// answering is optional
func getExisitingCorroboration(db *sql.DB, reportID, nonce string) (*CorroborationResponse, error) {
	var resp db.QueryRow(`
		SELECT id, report_id, nonce, answer, response_at 
		FROM corroboration_reponses
		WHERE report_id = ? AND nonce = ?
	`, reportID, nonce).Scan(&resp.ID, &resp.ReportID, resp.Nonce, &resp.Answer, &resp.RespondedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// records a response ("still-there" / "gone" / "not-sure" )
// from a nonce to a specific report 
func createCorroborationHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID := r.PathValue("id")
		var input struct {
			Nonce  string `json:"nonce"`
			Answer string `json:"answer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if input.Nonce == "" {
			http.Error(w, "nonce is required", http.StatusBadRequest)
			return
		}
		if input.Answer != "still-there" && input.Answer != "gone" && input.Answer != "not-sure" {
			http.Error(w, "answer must be one of: still-there, gone, not-sure", http.StatusBadRequest)
			return
		}
		// confirm report actually exists
		var exists int 
		err := db.QueryRow(`SELECT 1 FROM reports WHERE id = ?`, reportID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			http
		}
	}
}
