package knowledgestore

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckHealth_HTTP_OK(t *testing.T) {
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

	// Use the test server's TLS client so the self-signed cert is trusted.
	origClient := http.DefaultClient
	http.DefaultClient = srv.Client()
	defer func() { http.DefaultClient = origClient }()

	// httptest.NewTLSServer uses 127.0.0.1 — extract host from URL.
	creds := map[string]string{
		"HOST":    srv.Listener.Addr().String(),
		"API_KEY": "pk-test-key",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// checkHTTP builds https://HOST for pinecone, but our test server URL
	// includes the port already — call checkHTTP directly to control the URL.
	if err := checkHTTP(ctx, srv.URL, creds["API_KEY"]); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if gotKey != "pk-test-key" {
		t.Fatalf("expected Api-Key header 'pk-test-key', got %q", gotKey)
	}
}

func TestCheckHealth_TCP_OK(t *testing.T) {
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

	if err := CheckHealth(ctx, "mysql", creds); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckHealth_TCP_Refused(t *testing.T) {
	// Use a port that's very unlikely to be listening.
	creds := map[string]string{"HOST": "127.0.0.1", "PORT": "1"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := CheckHealth(ctx, "mysql", creds); err == nil {
		t.Fatal("expected error for refused connection")
	}
}

func TestCheckHealth_Timeout(t *testing.T) {
	// Use a non-routable address to trigger a timeout.
	creds := map[string]string{"HOST": "192.0.2.1", "PORT": "9999"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := CheckHealth(ctx, "mysql", creds)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCheckHealth_UnknownProvider_FallsBackToTCP(t *testing.T) {
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
