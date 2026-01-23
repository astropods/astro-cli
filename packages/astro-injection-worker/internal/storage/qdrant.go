package storage

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"github.com/postman/astro/packages/astro-injection-worker/internal/pipeline"
	"github.com/qdrant/go-client/qdrant"
)

// QdrantClient wraps the Qdrant vector database client
type QdrantClient struct {
	client         *qdrant.Client
	collectionName string
	vectorSize     uint64
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(host string, port int, collectionName string, vectorSize int) (*QdrantClient, error) {
	// Use gRPC port (6334) instead of REST port (6333)
	// The go-client defaults to gRPC protocol
	grpcPort := 6334
	if port == 6333 {
		log.Printf("Switching from REST port %d to gRPC port %d", port, grpcPort)
		port = grpcPort
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	log.Printf("Connected to Qdrant at %s:%d (gRPC mode)", host, port)

	return &QdrantClient{
		client:         client,
		collectionName: collectionName,
		vectorSize:     uint64(vectorSize),
	}, nil
}

// EnsureCollection creates the collection if it doesn't exist
func (q *QdrantClient) EnsureCollection(ctx context.Context) error {
	// Check if collection exists (with retry logic)
	var collections []string
	var lastErr error
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second // 1s, 4s, 9s, 16s
			log.Printf("Retry %d/%d connecting to Qdrant after %v...", attempt, maxRetries-1, backoff)
			time.Sleep(backoff)
		}

		collections, lastErr = q.client.ListCollections(ctx)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return fmt.Errorf("failed to list collections after %d retries: %w", maxRetries, lastErr)
	}

	for _, colName := range collections {
		if colName == q.collectionName {
			log.Printf("Collection %s already exists", q.collectionName)
			return nil
		}
	}

	// Create collection
	log.Printf("Creating collection: %s", q.collectionName)
	err := q.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: q.collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     q.vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	log.Printf("Successfully created collection: %s", q.collectionName)
	return nil
}

// UpsertChunks upserts chunks with embeddings to Qdrant
func (q *QdrantClient) UpsertChunks(ctx context.Context, chunks []pipeline.ChunkWithEmbedding) error {
	if len(chunks) == 0 {
		return nil
	}

	log.Printf("Upserting %d chunks to Qdrant collection: %s", len(chunks), q.collectionName)

	// Ensure collection exists
	if err := q.EnsureCollection(ctx); err != nil {
		return err
	}

	// Convert to Qdrant points
	points := make([]*qdrant.PointStruct, len(chunks))
	for i, chunk := range chunks {
		pointID := hashString(chunk.Chunk.ID)

		// Build payload
		payload := map[string]any{
			"text": chunk.Chunk.Content,
		}

		// Add all metadata fields
		isFirstChunk := false
		for key, value := range chunk.Chunk.Metadata {
			if key == "chunk_id" {
				// Track if this is the first chunk
				if value == "0" {
					isFirstChunk = true
				}
			}
			payload[key] = value
		}
		payload["is_first_chunk"] = isFirstChunk

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewIDNum(pointID),
			Vectors: qdrant.NewVectors(chunk.Embedding...),
			Payload: qdrant.NewValueMap(payload),
		}
	}

	// Upsert in batches of 100
	batchSize := 100
	for i := 0; i < len(points); i += batchSize {
		end := i + batchSize
		if end > len(points) {
			end = len(points)
		}

		batch := points[i:end]
		_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: q.collectionName,
			Points:         batch,
		})
		if err != nil {
			return fmt.Errorf("failed to upsert batch: %w", err)
		}

		log.Printf("Upserted batch %d/%d (%d points)", i/batchSize+1, (len(points)+batchSize-1)/batchSize, len(batch))
	}

	log.Printf("Successfully upserted %d points to Qdrant", len(points))
	return nil
}

// GetCollectionCount returns the total number of points in the collection
func (q *QdrantClient) GetCollectionCount(ctx context.Context) (int, error) {
	info, err := q.client.GetCollectionInfo(ctx, q.collectionName)
	if err != nil {
		return 0, fmt.Errorf("failed to get collection info: %w", err)
	}

	if info.PointsCount != nil {
		return int(*info.PointsCount), nil
	}
	return 0, nil
}

// Close closes the Qdrant client connection
func (q *QdrantClient) Close() error {
	if q.client != nil {
		return q.client.Close()
	}
	return nil
}

// hashString generates a uint64 hash from a string
func hashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
