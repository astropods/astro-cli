package knowledgestore

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
)

// mysqlSSRFNet is the net name the mysql driver dials through so the SSRF guard
// (ssrfDialer) applies to MySQL connections too.
const mysqlSSRFNet = "tcp-ssrf-guarded"

func init() {
	mysql.RegisterDialContext(mysqlSSRFNet, func(ctx context.Context, addr string) (net.Conn, error) {
		return ssrfDialer.DialContext(ctx, "tcp", addr)
	})
}

// ssrfDialer is used for every outbound dial the knowledge-store health check
// makes. HOST/PORT are attacker-supplied on the connect endpoint, so the
// Control hook rejects connections to non-public addresses. It runs against the
// *resolved* IP at dial time, so it also defeats DNS-rebinding (a hostname that
// resolves public on first lookup but private on the connect).
var ssrfDialer = &net.Dialer{
	Timeout: 5 * time.Second,
	Control: ssrfControl,
}

// ipAllowed decides whether the health check may dial a resolved IP. It is a
// package var (defaulting to the strict isPublicIP check) only so tests can
// permit loopback test servers; production never reassigns it.
var ipAllowed = isPublicIP

// healthHTTPClient is the client for HTTP-based provider health checks: it
// carries the SSRF-guarded dialer and uses no proxy (a proxy would bypass the
// IP check). A package var only so tests can substitute a client that trusts a
// local TLS test server.
var healthHTTPClient = &http.Client{Transport: &http.Transport{DialContext: ssrfDialer.DialContext}}

func ssrfControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: invalid address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf guard: unresolved address %q", host)
	}
	if !ipAllowed(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial non-public address %s", ip)
	}
	return nil
}

// isPublicIP reports whether ip is a globally-routable unicast address, i.e.
// not loopback/unspecified/link-local/multicast/private/CGNAT.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsPrivate() {
		return false
	}
	// Carrier-grade NAT 100.64.0.0/10 is not covered by IsPrivate; it is also
	// the cluster pod-network range, so block it to keep the health check from
	// reaching in-cluster pod IPs.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

// HumanizeHealthCheckError converts a raw CheckHealth error into a short,
// user-facing message free of driver, SQLSTATE, and network jargon. Callers
// should still log the raw error for debugging. Returns "" for a nil error.
func HumanizeHealthCheckError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "28p01"),
		strings.Contains(s, "password authentication failed"),
		strings.Contains(s, "authentication failed"),
		strings.Contains(s, "access denied"):
		return "Authentication failed — check the username and password."
	case strings.Contains(s, "ssrf guard"), strings.Contains(s, "non-public"):
		return "The host must be a publicly reachable address."
	case strings.Contains(s, "no such host"),
		strings.Contains(s, "server misbehaving"),
		strings.Contains(s, "lookup "):
		return "Couldn't find that host — check the hostname."
	case strings.Contains(s, "does not exist"):
		return "That database doesn't exist — check the database name."
	case strings.Contains(s, "certificate"), strings.Contains(s, "x509"), strings.Contains(s, "tls"):
		return "Couldn't establish a secure connection to the database."
	case strings.Contains(s, "network is unreachable"),
		strings.Contains(s, "no route to host"),
		strings.Contains(s, "connection refused"),
		strings.Contains(s, "i/o timeout"),
		strings.Contains(s, "timeout"),
		strings.Contains(s, "deadline exceeded"),
		strings.Contains(s, "refused"):
		return "Couldn't reach the database — check the host and port, and that it accepts connections from Astro."
	default:
		return "Couldn't connect to the database — check the connection details."
	}
}

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

// postgresConfig builds a pgx config from a credential map. Credentials are set
// on the parsed config directly rather than interpolated into the DSN: a
// username or password containing URL-special characters (%, @, :, /, #, …)
// would otherwise be mangled by URL parsing, a common source of spurious
// "password authentication failed" errors.
func postgresConfig(creds map[string]string) (*pgx.ConnConfig, error) {
	dsn := fmt.Sprintf("postgres://%s:%s/%s?sslmode=prefer",
		creds["HOST"], creds["PORT"], creds["DATABASE"],
	)
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	config.User = creds["USERNAME"]
	config.Password = creds["PASSWORD"]
	return config, nil
}

func checkPostgres(ctx context.Context, creds map[string]string) error {
	config, err := postgresConfig(creds)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	config.DialFunc = ssrfDialer.DialContext // SSRF guard on the user-supplied host
	conn, err := pgx.ConnectConfig(ctx, config)
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
	cfg.Net = mysqlSSRFNet // SSRF guard on the user-supplied host
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
		Dialer:   ssrfDialer.DialContext, // SSRF guard on the user-supplied host
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
	resp, err := healthHTTPClient.Do(req)
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
	conn, err := ssrfDialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port)) // SSRF guard
	if err != nil {
		return fmt.Errorf("tcp: %w", err)
	}
	conn.Close() //nolint:errcheck,gosec
	return nil
}
