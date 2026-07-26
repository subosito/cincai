package keyring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrBudgetExceeded is returned when a principal is over its rolling token budget.
var ErrBudgetExceeded = errors.New("usage limit exceeded")

// BudgetStatus is a snapshot of rolling-window usage vs limit.
type BudgetStatus struct {
	MaxTokens  int64         `json:"max_tokens"`
	UsedTokens int64         `json:"used_tokens"`
	Window     time.Duration `json:"-"`
	WindowSec  int64         `json:"window_sec"`
	// ResetAt is a rough UTC time when enough oldest usage may fall out of the window.
	// Zero if unknown (no chips yet or unlimited).
	ResetAt time.Time `json:"reset_at,omitempty"`
}

// CheckBudget returns nil if the principal may proceed, or ErrBudgetExceeded when over cap.
// Unlimited principals (MaxTokens==0) always pass.
func CheckBudget(ctx context.Context, ks KeyStore, p Principal) (BudgetStatus, error) {
	st := BudgetStatus{MaxTokens: p.BudgetMaxTokens}
	if !p.HasBudget() {
		return st, nil
	}
	win := p.BudgetWindowOrDefault()
	st.Window = win
	st.WindowSec = int64(win / time.Second)
	used, err := ks.BudgetUsed(ctx, p.KeyID, win)
	if err != nil {
		return st, err
	}
	st.UsedTokens = used
	if used >= p.BudgetMaxTokens {
		st.ResetAt = time.Now().UTC().Add(time.Minute) // coarse; refined by callers if needed
		return st, fmt.Errorf("%w: used %d / max %d tokens in %s", ErrBudgetExceeded, used, p.BudgetMaxTokens, win)
	}
	return st, nil
}

// RecordBudgetTokens appends measured tokens toward the principal's rolling budget.
// No-op when unlimited or tokens<=0.
func RecordBudgetTokens(ctx context.Context, ks KeyStore, p Principal, inputTokens, outputTokens int) error {
	if ks == nil || !p.HasBudget() {
		return nil
	}
	tok := int64(inputTokens) + int64(outputTokens)
	if tok <= 0 {
		return nil
	}
	return ks.BudgetRecord(ctx, p.KeyID, p.ID, tok)
}

// WriteBudgetExceeded writes an OpenAI-ish 429 JSON body for budget overruns.
func WriteBudgetExceeded(w http.ResponseWriter, st BudgetStatus) {
	w.Header().Set("Content-Type", "application/json")
	if !st.ResetAt.IsZero() {
		sec := int(time.Until(st.ResetAt).Seconds())
		if sec < 1 {
			sec = 60
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", sec))
	}
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":        "usage_limit_exceeded",
			"message":     "gateway key over rolling token budget",
			"limit":       st.MaxTokens,
			"used":        st.UsedTokens,
			"window_sec":  st.WindowSec,
			"reset_at":    formatReset(st.ResetAt),
		},
	})
}

func formatReset(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
