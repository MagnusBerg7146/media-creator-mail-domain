package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	api := domainClient{key: key, base: baseURL, http: &http.Client{}}
	http.HandleFunc("POST /creator-delivery/readiness", func(w http.ResponseWriter, r *http.Request) {
		var input deliveryInput
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
			http.Error(w, "invalid JSON request", http.StatusBadRequest)
			return
		}
		result, err := prepareDelivery(r.Context(), api, input)
		if err != nil {
			status := http.StatusBadRequest
			var remote *APIError
			if errors.As(err, &remote) {
				status = http.StatusBadGateway
				if remote.Status >= 400 && remote.Status < 500 {
					status = remote.Status
				}
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Print(err)
		}
	})
	log.Fatal(http.ListenAndServe(addr, nil))
}
