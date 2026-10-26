package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/marketplace-verification-gateway/internal/marketplace"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	publicURL := envOr("PUBLIC_URL", "http://localhost:8080")
	flow := marketplace.NewSignupFlow(&marketplace.InfraiEmail{APIKey: key}, publicURL)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			SellerID string                    `json:"seller_id"`
			Email    string                    `json:"email"`
			Assets   []marketplace.SellerAsset `json:"assets"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.SellerID == "" || input.Email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "seller_id and email are required"})
			return
		}
		messageID, err := flow.Signup(r.Context(), input.SellerID, input.Email, input.Assets)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *marketplace.InfraiError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				status = apiErr.Status
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_sent", "message_id": messageID})
	})
	mux.HandleFunc("GET /verify", func(w http.ResponseWriter, r *http.Request) {
		seller, err := flow.Verify(r.URL.Query().Get("token"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, seller)
	})
	mux.HandleFunc("POST /buyer-updates", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			SellerID string                  `json:"seller_id"`
			Update   marketplace.BuyerUpdate `json:"update"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		writeDecision(w, flow.PublishBuyerUpdate(input.SellerID, input.Update))
	})
	mux.HandleFunc("POST /order-handoffs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			SellerID string                   `json:"seller_id"`
			Handoff  marketplace.OrderHandoff `json:"handoff"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		writeDecision(w, flow.HandoffOrder(input.SellerID, input.Handoff))
	})
	log.Printf("marketplace signup listening on :8080 (public URL %s)", publicURL)
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func writeDecision(w http.ResponseWriter, err error) {
	if errors.Is(err, marketplace.ErrVerificationRequired) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
