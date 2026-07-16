package catalog

// DefaultModalityForWire returns a modality key implied by the wire alone when
// the catalog still has multiple modalities under one model id (should be rare
// after ExpandWireCollisions). Empty means “auto-select the sole candidate”.
func DefaultModalityForWire(wireID string) string {
	switch wireID {
	case WireOpenAIEmbed:
		return "embed"
	default:
		return ""
	}
}
