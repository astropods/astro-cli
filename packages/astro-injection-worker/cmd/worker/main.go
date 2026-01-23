package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/postman/astro/packages/astro-injection-worker/internal/pipeline"
	"github.com/postman/astro/packages/astro-injection-worker/internal/sources"
	"github.com/postman/astro/packages/astro-injection-worker/internal/state"
	"github.com/postman/astro/packages/astro-injection-worker/internal/storage"
	"github.com/postman/astro/packages/astro-injection-worker/pkg/config"
)

func main() {
	log.Println("Starting Astro Injection Worker...")

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Configuration loaded: source=%s, persistent=%v, dry_run=%v",
		cfg.SourceType, cfg.Persistent, cfg.DryRun)

	// Run injection
	if err := runInjection(cfg); err != nil {
		log.Fatalf("Injection failed: %v", err)
	}

	log.Println("Injection completed successfully")
}

func runInjection(cfg *config.Config) error {
	ctx := context.Background()
	startTime := time.Now()

	// Step 0: Load and check state (for persistent mode)
	var currentState *state.State
	var err error
	if cfg.Persistent {
		currentState, err = state.Load()
		if err != nil {
			return fmt.Errorf("failed to load state: %w", err)
		}
		log.Printf("State loaded: mode=%s, backfill_complete=%v, cursor=%s, issues_processed=%d",
			currentState.Mode, currentState.BackfillComplete, currentState.CurrentCursor, currentState.TotalIssuesProcessed)

		// Check rate limit
		if underLimit, waitTime := currentState.IsUnderRateLimit(); underLimit {
			log.Printf("Under rate limit, need to wait %v until %s", waitTime, currentState.RateLimitReset)
			return nil
		}
	}

	// Initialize embedder and qdrant clients once
	var embedder *pipeline.EmbedderClient
	var qdrantClient *storage.QdrantClient

	if !cfg.DryRun {
		embedder = pipeline.NewEmbedderClient(cfg.EmbedderURL)

		// Use collection config from environment (extracted from spec)
		collectionName := cfg.CollectionName
		vectorSize := cfg.VectorSize

		qdrantClient, err = storage.NewQdrantClient(cfg.TargetHost, cfg.TargetPort, collectionName, vectorSize)
		if err != nil {
			return fmt.Errorf("failed to create qdrant client: %w", err)
		}
		defer qdrantClient.Close()

		// Check for sync issues if in incremental mode
		if cfg.Persistent && currentState != nil && currentState.BackfillComplete {
			log.Printf("Checking database sync status...")
			actualCount, err := qdrantClient.GetCollectionCount(ctx)
			if err != nil {
				// Collection doesn't exist - definitely out of sync
				log.Printf("⚠️  Collection doesn't exist but backfill marked complete - resetting to backfill mode")
				currentState.Reset()
				if saveErr := currentState.Save(); saveErr != nil {
					log.Printf("Warning: Failed to save reset state: %v", saveErr)
				}
			} else {
				// Estimate expected chunks (avg ~1.6 chunks per issue based on typical data)
				expectedMinChunks := int(float64(currentState.TotalIssuesProcessed) * 0.8)

				if actualCount < expectedMinChunks {
					log.Printf("⚠️  Database out of sync: expected ~%d chunks, found %d in Qdrant",
						expectedMinChunks, actualCount)
					log.Printf("   Resetting to backfill mode to re-sync all data")
					currentState.Reset()
					if saveErr := currentState.Save(); saveErr != nil {
						log.Printf("Warning: Failed to save reset state: %v", saveErr)
					}
				} else {
					log.Printf("✓ Database in sync: %d points in Qdrant", actualCount)
				}
			}
		}
	}

	// Track totals
	totalDocs := 0
	totalChunks := 0

	// Step 1: Fetch and process using cursor-based pagination
	source, err := createSource(cfg)
	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	// Determine mode and cursor
	isIncremental := cfg.Persistent && currentState != nil && currentState.BackfillComplete
	cursor := ""
	lastSyncTime := ""

	if cfg.Persistent && currentState != nil {
		cursor = currentState.CurrentCursor
		lastSyncTime = currentState.LastSyncTimestamp
	}

	if isIncremental {
		log.Printf("Starting incremental sync from %s source (since: %s)...", cfg.SourceType, lastSyncTime)
	} else {
		log.Printf("Starting backfill from %s source...", cfg.SourceType)
	}

	batchNum := 1
	for {
		// Fetch one page using appropriate method
		var docs []sources.Document
		var nextCursor string
		var hasMore bool

		if isIncremental {
			docs, nextCursor, hasMore, err = source.FetchIncrementalWithCursor(ctx, lastSyncTime, cursor)
		} else {
			docs, nextCursor, hasMore, err = source.FetchPageWithCursor(ctx, cursor)
		}
		if err != nil {
			return fmt.Errorf("failed to fetch batch %d: %w", batchNum, err)
		}

		if len(docs) == 0 {
			log.Printf("No more documents in batch %d", batchNum)
			break
		}

		log.Printf("\n--- Processing Batch %d (%d documents) ---", batchNum, len(docs))
		batchStart := time.Now()

		// Process this batch immediately
		chunks := processDocuments(docs, cfg)
		log.Printf("  Generated %d chunks from batch %d", len(chunks), batchNum)

		if !cfg.DryRun {
			// Generate embeddings for this batch
			chunksWithEmbeddings, err := embedder.EmbedChunks(chunks)
			if err != nil {
				return fmt.Errorf("failed to generate embeddings for batch %d: %w", batchNum, err)
			}

			// Upsert to Qdrant immediately
			if err := qdrantClient.UpsertChunks(ctx, chunksWithEmbeddings); err != nil {
				return fmt.Errorf("failed to upsert batch %d to qdrant: %w", batchNum, err)
			}
		}

		// Update totals
		totalDocs += len(docs)
		totalChunks += len(chunks)

		log.Printf("  Batch %d complete in %v", batchNum, time.Since(batchStart))

		// Update state after each batch
		if cfg.Persistent && currentState != nil {
			currentState.TotalIssuesProcessed += len(docs)

			// Only save cursor for backfill mode
			// Incremental mode resets cursor each run to fetch latest updates
			if !isIncremental {
				currentState.CurrentCursor = nextCursor
			}

			currentState.LastSyncTimestamp = time.Now().UTC().Format(time.RFC3339)

			if err := currentState.Save(); err != nil {
				log.Printf("Warning: Failed to save state after batch %d: %v", batchNum, err)
			}
		}

		if !hasMore {
			log.Printf("Reached end of data after batch %d", batchNum)
			break
		}

		// Move to next cursor
		cursor = nextCursor
		batchNum++

		// Small delay between batches to be respectful
		time.Sleep(100 * time.Millisecond)
	}

	// Final summary
	duration := time.Since(startTime)
	log.Printf("\n=== Injection Summary ===")
	log.Printf("  Batches processed: %d", batchNum)
	log.Printf("  Documents fetched: %d", totalDocs)
	log.Printf("  Chunks created: %d", totalChunks)
	if !cfg.DryRun {
		log.Printf("  Embeddings generated: %d", totalChunks)
		log.Printf("  Points upserted: %d", totalChunks)

		// Get final count only if we upserted data
		if totalChunks > 0 {
			count, err := qdrantClient.GetCollectionCount(ctx)
			if err != nil {
				log.Printf("  Warning: Failed to get collection count: %v", err)
			} else {
				log.Printf("  Qdrant collection total: %d points", count)
			}
		}
	}
	log.Printf("  Total time: %v", duration)
	if totalDocs > 0 {
		log.Printf("  Avg time per document: %v", duration/time.Duration(totalDocs))
	}
	log.Printf("========================\n")

	// Mark backfill complete if needed
	if cfg.Persistent && currentState != nil && !currentState.BackfillComplete && !isIncremental {
		log.Println("Backfill complete - switching to incremental mode")
		currentState.MarkBackfillComplete()
		if err := currentState.Save(); err != nil {
			log.Printf("Warning: Failed to save state: %v", err)
		}
	}

	return nil
}

// PagedSource interface for sources that support cursor-based pagination
type PagedSource interface {
	FetchPageWithCursor(ctx context.Context, cursor string) ([]sources.Document, string, bool, error)
	FetchIncrementalWithCursor(ctx context.Context, since string, cursor string) ([]sources.Document, string, bool, error)
}

func createSource(cfg *config.Config) (PagedSource, error) {
	switch cfg.SourceType {
	case "github":
		repo, ok := cfg.SourceConfig["repo"].(string)
		if !ok {
			return nil, fmt.Errorf("repo not specified in source config")
		}

		source, err := sources.NewGitHubSource(cfg.GithubToken, repo)
		if err != nil {
			return nil, err
		}

		return source, nil

	default:
		return nil, fmt.Errorf("unsupported source type: %s", cfg.SourceType)
	}
}

func processDocuments(docs []sources.Document, cfg *config.Config) []pipeline.Chunk {
	// Extract max chunk size from pipeline config
	maxChunkSize := 1000
	for _, step := range cfg.Pipeline {
		if step.Step == "chunk" {
			if size, ok := step.Config["max_size"].(float64); ok {
				maxChunkSize = int(size)
			}
		}
	}

	processor := pipeline.NewProcessor(maxChunkSize)
	return processor.ChunkDocuments(docs)
}
