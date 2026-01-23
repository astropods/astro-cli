package pipeline

import (
	"fmt"
	"strings"

	"github.com/postman/astro/packages/astro-injection-worker/internal/sources"
)

// Chunk represents a document chunk
type Chunk struct {
	ID       string
	Content  string
	Metadata map[string]string
}

// Processor processes documents through the pipeline
type Processor struct {
	maxChunkSize int
}

// NewProcessor creates a new pipeline processor
func NewProcessor(maxChunkSize int) *Processor {
	if maxChunkSize == 0 {
		maxChunkSize = 1000 // default
	}
	return &Processor{
		maxChunkSize: maxChunkSize,
	}
}

// ChunkDocuments splits documents into chunks
func (p *Processor) ChunkDocuments(docs []sources.Document) []Chunk {
	var chunks []Chunk
	docsProcessed := 0
	logInterval := max(len(docs)/10, 10) // Log every 10% or every 10 docs

	fmt.Printf("Chunking %d documents (max chunk size: %d chars)...\n", len(docs), p.maxChunkSize)

	for _, doc := range docs {
		// Simple chunking strategy - split by paragraphs or max size
		content := doc.Content

		if len(content) <= p.maxChunkSize {
			// Document fits in one chunk
			chunkMeta := make(map[string]string)
			for k, v := range doc.Metadata {
				chunkMeta[k] = v
			}
			chunkMeta["chunk_id"] = "0"

			chunks = append(chunks, Chunk{
				ID:       doc.ID,
				Content:  content,
				Metadata: chunkMeta,
			})
		} else {
			// Split into multiple chunks
			paragraphs := strings.Split(content, "\n\n")
			currentChunk := ""
			chunkIndex := 0

			for _, para := range paragraphs {
				if len(currentChunk)+len(para)+2 > p.maxChunkSize {
					// Current chunk is full, save it
					if currentChunk != "" {
						chunks = append(chunks, Chunk{
							ID:       fmt.Sprintf("%s-chunk-%d", doc.ID, chunkIndex),
							Content:  currentChunk,
							Metadata: doc.Metadata,
						})
						chunkIndex++
						currentChunk = ""
					}
				}

				if currentChunk == "" {
					currentChunk = para
				} else {
					currentChunk += "\n\n" + para
				}
			}

			// Add remaining chunk
			if currentChunk != "" {
				chunkMeta := make(map[string]string)
				for k, v := range doc.Metadata {
					chunkMeta[k] = v
				}
				chunkMeta["chunk_id"] = fmt.Sprintf("%d", chunkIndex)

				chunks = append(chunks, Chunk{
					ID:       fmt.Sprintf("%s-chunk-%d", doc.ID, chunkIndex),
					Content:  currentChunk,
					Metadata: chunkMeta,
				})
			}
		}

		docsProcessed++

		// Log progress periodically
		if docsProcessed%logInterval == 0 || docsProcessed == len(docs) {
			progress := float64(docsProcessed) / float64(len(docs)) * 100
			fmt.Printf("  Chunked %d/%d documents (%.1f%%) - %d chunks created so far\n",
				docsProcessed, len(docs), progress, len(chunks))
		}
	}

	fmt.Printf("Chunking complete: %d documents → %d chunks\n", len(docs), len(chunks))
	return chunks
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Note: Embedding and upsert would be implemented here
// For now, this is a simplified version that focuses on chunking
