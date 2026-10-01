package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	client, err := NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	webhookURL, webhookSecret := os.Getenv("WEBHOOK_URL"), os.Getenv("WEBHOOK_SECRET")
	if webhookURL == "" || webhookSecret == "" {
		log.Fatal("WEBHOOK_URL and WEBHOOK_SECRET are required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /onboard-domain", func(w http.ResponseWriter, r *http.Request) {
		var input OnboardingRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid onboarding request", http.StatusBadRequest)
			return
		}
		result, err := client.Onboard(context.Background(), input, webhookURL, webhookSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /webhooks/domain-verification", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !validWebhook(body, r.Header.Get("X-Infrai-Signature"), webhookSecret) {
			http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
