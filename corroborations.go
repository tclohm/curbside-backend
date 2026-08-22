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
	RespondedAt	string `json:"Responded_at"`
}

// look at prior response for this
// answering is optional
func getExisitingCorroboration(db *sql.DB, reportID, nonce string) (*CorroborationResponse, error) {
	var resp CorroborationResponse
	err := db.QueryRow(`
		SELECT id, report_id, nonce, answer, responded_at 
		FROM corroboration_reponses
		WHERE report_id = ? AND nonce = ?
	`, reportID, nonce).Scan(&resp.ID, &resp.ReportID, &resp.Nonce, &resp.Answer, &resp.RespondedAt)

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
			http.Error(w, "report not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// nonce already answered
		existing, err := getExisitingCorroboration(db, reportID, input.Nonce)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existing)
			return
		}

		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := CorroborationResponse{
			ID:          id.String(),
			ReportID:    reportID,
			Nonce:       input.Nonce,
			Answer:      input.Answer,
			RespondedAt: time.Now().UTC().Format(time.RFC3339),
		}

		_, err = db.Exec(
			`INSERT INTO corroboration_responses (id, report_id, nonce, answer, responded_at)
			 VALUES (?, ?, ?, ?, ?)`,
			resp.ID, resp.ReportID, resp.Nonce, resp.Answer, resp.RespondedAt,
		)
		if err != nil {
			if isConstraintError(err) {
				// Known, accepted gap: two requests for the same brand-new
				// (report, nonce) pair could both pass the check above
				// before either inserts — a check-then-act race. Whichever
				// loses lands here; treat it the same as finding it above,
				// rather than erroring over what's really the same answer.
				existing, ferr := getExisitingCorroboration(db, reportID, input.Nonce)
				if ferr == nil && existing != nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(existing)
					return
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if input.Answer == "still-there" {
			// The increment happens inside the database, in one statement,
			// rather than "read count in Go, add 1, write it back" — that
			// read-modify-write pattern is a classic race under concurrent
			// requests (two requests could both read 1 and both write 2,
			// losing an increment). Expressing it as `count = count + 1`
			// makes SQLite do the read-and-write atomically.
			if _, err := db.Exec(
				`UPDATE reports SET corroboration_count = corroboration_count + 1 WHERE id = ?`,
				reportID,
			); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Guarded by status = 'submitted' so this only ever fires once,
			// no matter how many "still-there" responses arrive after the
			// threshold, or in what order concurrent requests land.
			if _, err := db.Exec(
				`UPDATE reports SET status = 'under_review'
				 WHERE id = ? AND status = 'submitted' AND corroboration_count >= 2`,
				reportID,
			); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}
