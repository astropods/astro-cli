"""
{{.Name}} - Webhook Ingestion Server

An HTTP server that receives webhook events and ingests data
into your knowledge stores.

Environment variables available:
  PORT - Server port (default: 3001)
{{- if .HasKnowledge "qdrant"}}
  QDRANT_URL - Qdrant vector database connection URL
{{- end}}
{{- if .HasKnowledge "redis"}}
  REDIS_URL - Redis key-value store connection URL
{{- end}}
{{- if .HasKnowledge "neo4j"}}
  NEO4J_URL - Neo4j graph database connection URL
{{- end}}
{{- range .Integrations}}
{{- if eq . "github"}}
  GITHUB_TOKEN - GitHub API token for fetching repository data
{{- end}}
{{- end}}
"""

import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(os.environ.get("PORT", 3001))


class WebhookHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok")
        else:
            self._not_found()

    def do_POST(self):
        if self.path == "/webhook":
            try:
                length = int(self.headers.get("Content-Length", 0))
                body = json.loads(self.rfile.read(length))
                print(f"Webhook received: {json.dumps(body)[:200]}")

                #
                # TODO: Process the webhook payload and ingest into knowledge stores
                #
{{- if .HasKnowledge "qdrant"}}
                # qdrant_url = os.environ.get("QDRANT_URL")
                # Generate embeddings and upsert into Qdrant
{{- end}}
{{- if .HasKnowledge "redis"}}
                # redis_url = os.environ.get("REDIS_URL")
                # Cache processed data in Redis
{{- end}}
{{- if .HasKnowledge "neo4j"}}
                # neo4j_url = os.environ.get("NEO4J_URL")
                # Store relationships in Neo4j
{{- end}}

                self.send_response(202)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"status": "accepted"}).encode())
            except Exception as e:
                print(f"Error processing webhook: {e}")
                self.send_response(400)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": "invalid payload"}).encode())
        else:
            self._not_found()

    def _not_found(self):
        self.send_response(404)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"error": "not found"}).encode())

    def log_message(self, format, *args):
        pass  # suppress default access logs


if __name__ == "__main__":
    server = HTTPServer(("", PORT), WebhookHandler)
    print(f"Webhook ingestion server listening on port {PORT}")
    server.serve_forever()
