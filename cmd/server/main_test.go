package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// itemsDir is the sample test form, relative to this package's directory.
const itemsDir = "../../testdata/items"

// runTimeout is how long a test waits for run to return before failing.
const runTimeout = 5 * time.Second

// syncBuffer is a bytes.Buffer that is safe to write from the server's
// goroutines while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startRun calls run in a goroutine and returns a channel that receives its
// result.
func startRun(ctx context.Context, args []string, logOut io.Writer) <-chan error {
	done := make(chan error, 1)
	go func() { done <- run(ctx, args, logOut) }()
	return done
}

// waitRun returns run's result, failing the test if it takes longer than
// runTimeout.
func waitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(runTimeout):
		t.Fatalf("run did not return within %v", runTimeout)
		return nil
	}
}

var listeningAddr = regexp.MustCompile(`msg=listening addr=(\S+)`)

// waitListening waits for run to log the address it listens on and returns
// it.
func waitListening(t *testing.T, logs *syncBuffer, done <-chan error) string {
	t.Helper()
	deadline := time.Now().Add(runTimeout)
	for time.Now().Before(deadline) {
		if m := listeningAddr.FindStringSubmatch(logs.String()); m != nil {
			return m[1]
		}
		select {
		case err := <-done:
			t.Fatalf("run returned %v before listening; log:\n%s", err, logs)
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatalf("run did not log a listening address; log:\n%s", logs)
	return ""
}

// TestRunServesAndShutsDown starts the server on a free port, makes one real
// request over TCP, then cancels the context and checks that run shuts down
// cleanly.
func TestRunServesAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var logs syncBuffer
	done := startRun(ctx, []string{"-addr", "127.0.0.1:0", "-items", itemsDir}, &logs)

	addr := waitListening(t, &logs, done)
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status = %d, want 200", resp.StatusCode)
	}

	cancel()
	if err := waitRun(t, done); err != nil {
		t.Fatalf("run = %v, want nil after the context is cancelled", err)
	}
	log := logs.String()
	for _, want := range []string{"path=/healthz status=200", "msg=\"shutting down\"", "msg=stopped"} {
		if !strings.Contains(log, want) {
			t.Errorf("log does not contain %q:\n%s", want, log)
		}
	}
}

func TestRunHelp(t *testing.T) {
	var logs syncBuffer
	err := waitRun(t, startRun(t.Context(), []string{"-h"}, &logs))
	if err != nil {
		t.Errorf("run -h = %v, want nil", err)
	}
	if !strings.Contains(logs.String(), "-items") {
		t.Errorf("run -h printed %q, want the usage", logs.String())
	}
}

func TestRunErrors(t *testing.T) {
	// A port that is already taken.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer taken.Close()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown flag", []string{"-nope"}, "-nope"},
		{"missing items folder", []string{"-addr", "127.0.0.1:0", "-items", "no-such-folder"}, "load items"},
		{"invalid address", []string{"-addr", "127.0.0.1:99999", "-items", itemsDir}, "listen"},
		{"address in use", []string{"-addr", taken.Addr().String(), "-items", itemsDir}, "listen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs syncBuffer
			err := waitRun(t, startRun(t.Context(), tt.args, &logs))
			if err == nil {
				t.Fatalf("run succeeded, want an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestNewServer pins the header timeout, which stops a client from holding a
// connection open by sending its headers slowly.
func TestNewServer(t *testing.T) {
	srv := newServer(http.NotFoundHandler())
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.Handler == nil {
		t.Error("Handler is nil, want the handler passed in")
	}
}
