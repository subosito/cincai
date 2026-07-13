package mistral

import "testing"

func TestExtractOcrConfidence(t *testing.T) {
	data := map[string]any{
		"pages": []any{
			map[string]any{
				"confidence_scores": map[string]any{
					"average_page_confidence_score": 0.8469325833034246,
					"minimum_page_confidence_score": 0.2953792585394389,
					"word_confidence_scores": []any{
						map[string]any{"confidence": 0.91, "text": "SHEET"},
						map[string]any{"confidence": 0.64, "text": "Date"},
						map[string]any{"confidence": 0.30, "text": "Person:"},
					},
				},
			},
		},
	}
	got := extractOcrConfidence(data)
	if got == nil {
		t.Fatal("nil summary")
	}
	if got.AveragePage < 0.84 || got.AveragePage > 0.85 {
		t.Fatalf("average_page=%v", got.AveragePage)
	}
	if got.MinimumPage < 0.29 || got.MinimumPage > 0.30 {
		t.Fatalf("minimum_page=%v", got.MinimumPage)
	}
	if got.WordCount != 3 {
		t.Fatalf("word_count=%d", got.WordCount)
	}
	if got.LowWordRatio < 0.66 || got.LowWordRatio > 0.67 {
		t.Fatalf("low_word_ratio=%v", got.LowWordRatio)
	}
}