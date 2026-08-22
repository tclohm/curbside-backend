package main

import (
	"database/sql"
	"log"
	"net/http"
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

  return mux
}

func main() {
	db, err := openDB("curbside.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Printf("[INFO] - reports table ready")

	log.Printf("[INFO] - listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", newMux(db)))
}
