/**
 * {{.Name}} - Data Ingestion Pipeline
 *
 * This script handles data ingestion into your knowledge stores.
 * Run with: bun run ingest
 *
 * Environment variables available:
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}
 *   QDRANT_URL - Qdrant vector database connection URL
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}
 *   REDIS_URL - Redis key-value store connection URL
{{- end}}
{{- range .Integrations}}
{{- if eq . "github"}}
 *   GITHUB_TOKEN - GitHub API token for fetching repository data
{{- end}}
{{- end}}
 */

async function main() {
  console.log("Starting ingestion pipeline for {{.Name}}...");

  //
  // TODO: Implement your data ingestion logic here
  //
  // Common ingestion patterns:
  //
  // 1. Fetch data from external sources (APIs, databases, files)
  // 2. Process and chunk the data for embedding
  // 3. Generate embeddings using your model
  // 4. Store in your knowledge base (vector store, key-value store)
  //
{{- if or (eq .Knowledge "vector") (eq .Knowledge "both")}}

  // Example: Ingest documents into Qdrant
  // const qdrantUrl = process.env.QDRANT_URL;
  //
  // const documents = [
  //   { id: "1", content: "Document content here...", metadata: {} },
  // ];
  //
  // for (const doc of documents) {
  //   // 1. Generate embedding for the document
  //   // 2. Upsert into Qdrant collection
  // }
{{- end}}
{{- if or (eq .Knowledge "kv") (eq .Knowledge "both")}}

  // Example: Cache data in Redis
  // const redisUrl = process.env.REDIS_URL;
  //
  // Store frequently accessed data or pre-computed results
{{- end}}

  console.log("Ingestion complete!");
}

main().catch(console.error);
