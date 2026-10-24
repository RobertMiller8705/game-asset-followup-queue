package followup

import "time"

type Asset struct {
	AssetID     string    `json:"asset_id"`
	PlayerID    string    `json:"player_id"`
	EventID     string    `json:"event_id"`
	SubmittedAt time.Time `json:"submitted_at"`
	ReviewAfter time.Time `json:"review_after"`
}

type Decision string

const (
	Wait   Decision = "wait"
	Review Decision = "review"
)

// Decide keeps the timing rule separate from queue delivery details.
func Decide(now time.Time, asset Asset) Decision {
	if now.Before(asset.ReviewAfter) {
		return Wait
	}
	return Review
}
