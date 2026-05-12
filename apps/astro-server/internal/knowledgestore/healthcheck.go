package knowledgestore

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
)

// CheckHealth attempts to reach an external knowledge store using the plaintext
// credential map. Returns nil on success, a descriptive error on failure.
// The caller should set a timeout on ctx (recommend 5s).
func CheckHealth(ctx context.Context, provider string, creds map[string]string) error {
	host := creds["HOST"]
	port := creds["PORT"]

	switch provider {
	case "postgres":
		return checkPostgres(ctx, creds)
	case "mysql":
		return checkMySQL(ctx, creds)
	case "redis":
		return checkRedis(ctx, host, port, creds["PASSWORD"])
	case "qdrant":
		return checkHTTP(ctx, fmt.Sprintf("http://%s:%s/healthz", host, port), "")
	case "neo4j":
		// Neo4j exposes an HTTP endpoint on port 7474.
		if port == "7474" {
			return checkHTTP(ctx, fmt.Sprintf("http://%s:%s/", host, port), "")
		}
		// Non-standard port (e.g. bolt 7687) — fall back to TCP dial.
		return checkTCP(ctx, host, port)
	case "pinecone":
		return checkHTTP(ctx, fmt.Sprintf("https://%s", host), creds["API_KEY"])
	default:
		return checkTCP(ctx, host, port)
	}
}

func checkPostgres(ctx context.Context, creds map[string]string) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=prefer",
		creds["USERNAME"], creds["PASSWORD"],
		creds["HOST"], creds["PORT"], creds["DATABASE"],
	)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer conn.Close(ctx) //nolint:errcheck
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}

func checkMySQL(ctx context.Context, creds map[string]string) error {
	cfg := mysql.NewConfig()
	cfg.User = creds["USERNAME"]
	cfg.Passwd = creds["PASSWORD"]
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(creds["HOST"], creds["PORT"])
	cfg.DBName = creds["DATABASE"]
	cfg.Timeout = 5 * time.Second
	cfg.AllowNativePasswords = true
	cfg.TLSConfig = "preferred" // try TLS first, fall back to plaintext — matches postgres sslmode=prefer

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}
	defer db.Close() //nolint:errcheck
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("mysql ping: %w", err)
	}
	return nil
}

func checkRedis(ctx context.Context, host, port, password string) error {
	client := goredis.NewClient(&goredis.Options{
		Addr:     net.JoinHostPort(host, port),
		Password: password,
	})
	defer client.Close() //nolint:errcheck
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	return nil
}

func checkHTTP(ctx context.Context, url, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Api-Key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http: unexpected status %d from %s", resp.StatusCode, url)
	}
	return nil
}

func checkTCP(ctx context.Context, host, port string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("tcp: %w", err)
	}
	conn.Close() //nolint:errcheck,gosec
	return nil
}
