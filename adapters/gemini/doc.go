// Package gemini converts between OpenAI chat.completions and Gemini
// generateContent request/response shapes (tools, multimodal parts,
// thought_signature, thinking budget).
//
// Used by the Vertex adapter and by product adapters (e.g. Cloud Code) that
// speak the same generateContent wire with a different base URL or path.
package gemini
