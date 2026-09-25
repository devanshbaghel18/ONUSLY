package main

import (
	"encoding/json"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
	})

	http.ListenAndServe(":8080", nil)
}
