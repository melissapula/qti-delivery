// Command server serves the QTI delivery API over HTTP.
//
// Usage:
//
//	server [-addr :8080] [-items testdata/items]
//
// It loads every item in the -items folder, in file-name order, as one test
// form, keeps sessions in memory, and logs each request to stderr. Ctrl+C
// shuts it down gracefully: it stops accepting connections and waits up to
// 10 seconds for requests already in progress.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/melissapula/qti-delivery/internal/api"
	"github.com/melissapula/qti-delivery/internal/qti"
	"github.com/melissapula/qti-delivery/internal/session"
)

const (
	// readHeaderTimeout limits how long a client may take to send its
	// request headers, so a slow client cannot hold a connection open.
	readHeaderTimeout = 5 * time.Second
	// shutdownTimeout limits how long shutdown waits for requests in
	// progress.
	shutdownTimeout = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Stderr)
	stop() // os.Exit skips deferred calls, so stop before it.
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses args, loads the items and serves the API until ctx is
// cancelled, then shuts down gracefully. Logs and usage text go to logOut.
// It returns nil after a clean shutdown or for -h, and an error if the
// arguments are invalid, the items cannot be loaded, or the server cannot
// listen or fails while serving.
func run(ctx context.Context, args []string, logOut io.Writer) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(logOut)
	addr := flags.String("addr", ":8080", "address to listen on")
	itemsDir := flags.String("items", "testdata/items", "folder of QTI item XML files that make up the test form")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	log := slog.New(slog.NewTextHandler(logOut, nil))
	items, err := qti.LoadDir(*itemsDir)
	if err != nil {
		return fmt.Errorf("load items: %w", err)
	}

	// Listening before serving reports a bad or busy address here, before
	// the "listening" log line, and lets -addr :0 pick a free port.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	srv := newServer(api.New(session.NewStore(items), log))
	serveErr := make(chan error, 1) // buffered, so Serve's goroutine never blocks
	go func() { serveErr <- srv.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String(), "items", len(items))

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down")
	// ctx is already cancelled, so the shutdown deadline needs a fresh one.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	// Serve returns ErrServerClosed once Shutdown starts; anything else is a
	// real failure.
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	log.Info("stopped")
	return nil
}

// newServer returns an HTTP server for handler with the header timeout set.
func newServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
}
