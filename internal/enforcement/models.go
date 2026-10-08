package enforcement

// TargetItem represents a single goal's active enforcement targets.
type TargetItem struct {
	GoalID    string   `json:"goalId"`
	GoalTitle string   `json:"goalTitle"`
	Status    string   `json:"status"`
	Apps      []string `json:"apps"`
	Domains   []string `json:"domains"`

	// unexported field for ordering
	createdAt string `json:"-"`
}

// BlocklistResponse represents the complete payload for GET /enforcement/blocklist.
type BlocklistResponse struct {
	UserID      string       `json:"userId"`
	Version     string       `json:"version"`
	GeneratedAt string       `json:"generatedAt"`
	Targets     []TargetItem `json:"targets"`
}
