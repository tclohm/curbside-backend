package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"
)

func newMux(db *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("Get /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /reports", createReportHandler(db))
  mux.HandleFunc("GET /reports", listReportsHandler(db))
  mux.HandleFunc("GET /reports/{id}", getReportHandler(db))
  mux.HandleFunc("POST /reports/{id}/corroborations", createCorroborationHandler(db))
	mux.HandleFunc("POST /photos", createPendingUploadHandler(db))

  return mux
}

func main() {
	db, err := openDB("curbside.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Printf("[INFO] - reports table ready")

	if err := os.MkdirAll(pendingUploadsDir, 0755); err != nil {
		log.Fatal(err)
	}

	log.Printf("[INFO] - %s ready", pendingUploadsDir)

	startExpirySweeper(db, time.Minute)
	log.Printf("[INFO] - expiry sweeper running (every 1m, TTL 8m)")

	log.Printf("[INFO] - listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", newMux(db)))
}
