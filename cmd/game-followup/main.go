package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/game-asset-followup/internal/followup"
	"example.com/game-asset-followup/internal/infrai"
)

type service struct {
	queue *infrai.Client
	now   func() time.Time
}

type submission struct {
	AssetID    string `json:"asset_id"`
	PlayerID   string `json:"player_id"`
	EventID    string `json:"event_id"`
	DelayHours int    `json:"delay_hours"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	svc := &service{queue: infrai.New(key), now: time.Now}

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "serve":
		mux := http.NewServeMux()
		mux.HandleFunc("POST /assets", svc.submitAsset)
		mux.HandleFunc("POST /work/run", svc.runWorker)
		addr := envOr("LISTEN_ADDR", ":8080")
		log.Printf("game follow-up service listening on %s", addr)
		log.Fatal(http.ListenAndServe(addr, mux))
	case "run-once":
		reviewed, waiting, err := svc.processBatch(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("reviewed=%d waiting=%d\n", reviewed, waiting)
	default:
		log.Fatalf("usage: game-followup [serve|run-once]")
	}
}

func (s *service) submitAsset(w http.ResponseWriter, r *http.Request) {
	var in submission
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.AssetID == "" || in.PlayerID == "" || in.EventID == "" || in.DelayHours < 1 {
		http.Error(w, "asset_id, player_id, event_id, and a positive delay_hours are required", http.StatusBadRequest)
		return
	}
	now := s.now().UTC()
	asset := followup.Asset{
		AssetID: in.AssetID, PlayerID: in.PlayerID, EventID: in.EventID,
		SubmittedAt: now, ReviewAfter: now.Add(time.Duration(in.DelayHours) * time.Hour),
	}
	if err := s.queue.Publish(r.Context(), asset, publishKey(asset)); err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, asset)
}

func (s *service) runWorker(w http.ResponseWriter, r *http.Request) {
	reviewed, waiting, err := s.processBatch(r.Context())
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"reviewed": reviewed, "waiting": waiting})
}

func (s *service) processBatch(ctx context.Context) (reviewed, waiting int, err error) {
	messages, err := s.queue.Consume(ctx, 10, 60)
	if err != nil {
		return 0, 0, err
	}
	for _, message := range messages {
		var asset followup.Asset
		if err := json.Unmarshal(message.Payload, &asset); err != nil {
			return reviewed, waiting, fmt.Errorf("decode queued asset: %w", err)
		}
		if followup.Decide(s.now().UTC(), asset) == followup.Wait {
			waiting++
			continue
		}
		log.Printf("moderation queued asset_id=%s player_id=%s event_id=%s", asset.AssetID, asset.PlayerID, asset.EventID)
		if err := s.queue.Ack(ctx, message.MessageID); err != nil {
			return reviewed, waiting, err
		}
		reviewed++
	}
	return reviewed, waiting, nil
}

func publishKey(asset followup.Asset) string {
	sum := sha256.Sum256([]byte(asset.AssetID + "\x00" + asset.EventID + "\x00" + asset.ReviewAfter.Format(time.RFC3339Nano)))
	return "asset-followup-" + hex.EncodeToString(sum[:])
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		writeJSON(w, apiErr.Status, map[string]any{"error": apiErr.Detail})
		return
	}
	http.Error(w, "queue request failed", http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
