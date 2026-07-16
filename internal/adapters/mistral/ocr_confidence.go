package mistral

// OcrConfidenceSummary aggregates Mistral OCR 4 per-page confidence_scores.
type OcrConfidenceSummary struct {
	AveragePage  float64 `json:"average_page"`
	MinimumPage  float64 `json:"minimum_page"`
	LowWordRatio float64 `json:"low_word_ratio"`
	WordCount    int     `json:"word_count"`
}

const ocrLowWordConfidenceThreshold = 0.65

func extractOcrConfidence(ocrData map[string]any) *OcrConfidenceSummary {
	pages, ok := ocrData["pages"].([]any)
	if !ok || len(pages) == 0 {
		return nil
	}

	var pageAvgs, pageMins []float64
	var lowWords, totalWords int

	for _, raw := range pages {
		page, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cs, ok := page["confidence_scores"].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := cs["average_page_confidence_score"].(float64); ok {
			pageAvgs = append(pageAvgs, v)
		}
		if v, ok := cs["minimum_page_confidence_score"].(float64); ok {
			pageMins = append(pageMins, v)
		}
		words, _ := cs["word_confidence_scores"].([]any)
		for _, rawWord := range words {
			word, ok := rawWord.(map[string]any)
			if !ok {
				continue
			}
			conf, ok := word["confidence"].(float64)
			if !ok {
				continue
			}
			totalWords++
			if conf < ocrLowWordConfidenceThreshold {
				lowWords++
			}
		}
	}

	if len(pageAvgs) == 0 && len(pageMins) == 0 && totalWords == 0 {
		return nil
	}

	out := &OcrConfidenceSummary{WordCount: totalWords}
	if len(pageAvgs) > 0 {
		out.AveragePage = meanFloat(pageAvgs)
	}
	if len(pageMins) > 0 {
		out.MinimumPage = minFloat(pageMins)
	}
	if totalWords > 0 {
		out.LowWordRatio = float64(lowWords) / float64(totalWords)
	}
	return out
}

func meanFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func minFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	min := vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
	}
	return min
}
