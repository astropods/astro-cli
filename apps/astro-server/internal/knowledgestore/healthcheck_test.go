package knowledgestore

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// allowLoopbackForTest relaxes the SSRF dial guard so the loopback httptest
// servers used below are reachable. Restored automatically at test end.
func allowLoopbackForTest(t *testing.T) {
	t.Helper()
	orig := ipAllowed
	ipAllowed = func(net.IP) bool { return true }
	t.Cleanup(func() { ipAllowed = orig })
}

func TestCheckHealth_HTTP_OK(t *testing.T) {
	allowLoopbackForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	creds := map[string]string{"HOST": host, "PORT": port}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := CheckHealth(ctx, "qdrant", creds); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckHealth_HTTP_ServerError(t *testing.T) {
	allowLoopbackForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	creds := map[string]string{"HOST": host, "PORT": port}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := CheckHealth(ctx, "qdrant", creds); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestCheckHealth_HTTP_Pinecone_APIKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Use the test server's client so the self-signed cert is trusted. It also
	// has no SSRF Control hook, so the loopback address is reachable.
	origClient := healthHTTPClient
	healthHTTPClient = srv.Client()
	defer func() { healthHTTPClient = origClient }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// checkHTTP builds https://HOST for pinecone, but our test server URL
	// includes the port already — call checkHTTP directly to control the URL.
	if err := checkHTTP(ctx, srv.URL, "pk-test-key"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if gotKey != "pk-test-key" {
		t.Fatalf("expected Api-Key header 'pk-test-key', got %q", gotKey)
	}
}

func TestCheckHealth_TCP_Refused(t *testing.T) {
	allowLoopbackForTest(t)
	// Use a port that's very unlikely to be listening.
	creds := map[string]string{"HOST": "127.0.0.1", "PORT": "1"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := CheckHealth(ctx, "unknown-provider", creds); err == nil {
		t.Fatal("expected error for refused connection")
	}
}

func TestCheckHealth_Timeout(t *testing.T) {
	// Use a non-routable (but public, so not SSRF-blocked) address to trigger a
	// timeout. 192.0.2.0/24 is TEST-NET-1 — not loopback/private/link-local.
	creds := map[string]string{"HOST": "192.0.2.1", "PORT": "9999"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := CheckHealth(ctx, "unknown-provider", creds)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCheckHealth_UnknownProvider_FallsBackToTCP(t *testing.T) {
	allowLoopbackForTest(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	creds := map[string]string{"HOST": host, "PORT": port}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := CheckHealth(ctx, "unknown-provider", creds); err != nil {
		t.Fatalf("expected nil for unknown provider with reachable TCP, got %v", err)
	}
}

// --- SSRF guard tests ---

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// Public — allowed.
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"192.0.2.1", true}, // TEST-NET-1: non-routable but not private/reserved-unicast
		{"2606:4700:4700::1111", true},
		// Loopback / unspecified.
		{"127.0.0.1", false},
		{"::1", false},
		{"0.0.0.0", false},
		// RFC1918 private.
		{"10.0.0.1", false},
		{"172.16.5.4", false},
		{"192.168.1.1", false},
		// Link-local incl. cloud metadata.
		{"169.254.169.254", false},
		{"fe80::1", false},
		// IPv6 ULA.
		{"fc00::1", false},
		{"fd12:3456::1", false},
		// CGNAT / cluster pod range 100.64.0.0/10.
		{"100.64.0.1", false},
		{"100.127.255.254", false},
		{"100.63.255.255", true}, // just below the /10
		{"100.128.0.1", true},    // just above the /10
		// Multicast.
		{"224.0.0.1", false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", tc.ip)
		}
		if got := isPublicIP(ip); got != tc.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestSSRFControl(t *testing.T) {
	cases := []struct {
		address string
		wantErr bool
	}{
		{"8.8.8.8:443", false},
		{"127.0.0.1:5432", true},
		{"10.0.0.5:6379", true},
		{"169.254.169.254:80", true},
		{"100.64.0.1:5432", true},
		{"not-an-ip:443", true}, // Control receives resolved IPs; a hostname here is rejected
		{"8.8.8.8", true},       // missing port
	}
	for _, tc := range cases {
		err := ssrfControl("tcp", tc.address, nil)
		if (err != nil) != tc.wantErr {
			t.Errorf("ssrfControl(%q) err=%v, wantErr=%v", tc.address, err, tc.wantErr)
		}
	}
}

// TestCheckHealth_SSRF_BlocksLoopback proves the guard blocks a *reachable*
// loopback target (the classic SSRF/oracle case) — without the test seam.
func TestCheckHealth_SSRF_BlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	creds := map[string]string{"HOST": host, "PORT": port}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := CheckHealth(ctx, "qdrant", creds)
	if err == nil {
		t.Fatal("expected SSRF guard to block loopback, got nil")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("expected an SSRF-guard error, got %v", err)
	}
}

// TestCheckHealth_SSRF_BlocksMetadata covers the cloud metadata IP for a raw
// TCP provider path (rejected before any dial).
func TestCheckHealth_SSRF_BlocksMetadata(t *testing.T) {
	creds := map[string]string{"HOST": "169.254.169.254", "PORT": "80"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := CheckHealth(ctx, "unknown-provider", creds)
	if err == nil {
		t.Fatal("expected SSRF guard to block link-local metadata IP, got nil")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("expected an SSRF-guard error, got %v", err)
	}
}
