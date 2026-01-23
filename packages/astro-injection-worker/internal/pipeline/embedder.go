package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EmbedderClient calls the embedder service to generate vectors
type EmbedderClient struct {
	baseURL string
	client  *http.Client
}

// EmbedRequest is sent to the embedder service
type EmbedRequest struct {
	Texts []string `json:"texts"`
}

// EmbedResponse is returned from the embedder service
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// NewEmbedderClient creates a new embedder client
func NewEmbedderClient(baseURL string) *EmbedderClient {
	return &EmbedderClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// EmbedChunks generates embeddings for chunks
func (e *EmbedderClient) EmbedChunks(chunks []Chunk) ([]ChunkWithEmbedding, error) {
	if len(chunks) == 0 {
		return []ChunkWithEmbedding{}, nil
	}

	fmt.Printf("Generating embeddings for %d chunks via %s...\n", len(chunks), e.baseURL)
	startTime := time.Now()

	// Extract texts
	texts := make([]string, len(chunks))
	totalChars := 0
	for i, chunk := range chunks {
		texts[i] = chunk.Content
		totalChars += len(chunk.Content)
	}
	avgChars := totalChars / len(chunks)
	fmt.Printf("  Total characters: %d (avg: %d per chunk)\n", totalChars, avgChars)

	// Call embedder service
	reqBody := EmbedRequest{Texts: texts}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	fmt.Printf("  Sending request to embedder (payload: %.1f KB)...\n", float64(len(jsonData))/1024)

	// Retry with exponential backoff in case embedder is not ready yet
	var resp *http.Response
	var lastErr error
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second // 1s, 4s, 9s, 16s
			fmt.Printf("  Retry %d/%d after %v...\n", attempt, maxRetries-1, backoff)
			time.Sleep(backoff)
		}

		resp, lastErr = e.client.Post(
			e.baseURL+"/embed",
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("failed to call embedder after %d retries: %w", maxRetries, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedder returned status %d", resp.StatusCode)
	}

	var embedResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embedResp.Embeddings) != len(chunks) {
		return nil, fmt.Errorf("embedding count mismatch: got %d, expected %d",
			len(embedResp.Embeddings), len(chunks))
	}

	// Attach embeddings to chunks
	result := make([]ChunkWithEmbedding, len(chunks))
	vectorDim := 0
	if len(embedResp.Embeddings) > 0 {
		vectorDim = len(embedResp.Embeddings[0])
	}
	for i := range chunks {
		result[i] = ChunkWithEmbedding{
			Chunk:     chunks[i],
			Embedding: embedResp.Embeddings[i],
		}
	}

	duration := time.Since(startTime)
	chunksPerSec := float64(len(chunks)) / duration.Seconds()
	fmt.Printf("Embedding complete: %d vectors (dim: %d) in %v (%.1f chunks/sec)\n",
		len(result), vectorDim, duration, chunksPerSec)

	return result, nil
}

// ChunkWithEmbedding represents a chunk with its vector embedding
type ChunkWithEmbedding struct {
	Chunk     Chunk
	Embedding []float32
}
