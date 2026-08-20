package main

import (
	"log"
	"net/http"
)

func main() {
	db, err := openDB("curbside.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Printf("reports table ready")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
