// Command packrat serves the Packrat packing-list web application.
//
// Usage:
//
//	packrat [-addr :8082] [-db postgres:///packrat?host=/var/run/postgresql] [-static DIR]
//	packrat -version
//
// The flags default to the environment variables PACKRAT_ADDR,
// PACKRAT_DATABASE_URL and PACKRAT_STATIC_DIR.
//
// On start the schema is brought up to date from the migrations embedded in
// the binary (internal/db/migrations).
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"packrat/internal/api"
	"packrat/internal/db"
	"packrat/internal/store"
)

//go:embed static
var staticFS embed.FS

// Version is the semantic version of this release and VersionDate the day it
// was cut. build is the git description, set via -ldflags "-X main.build=...".
const (
	Version     = "0.1.0"
	VersionDate = "2026-11-16"
)

var build = "dev"

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", envOr("PACKRAT_ADDR", ":8082"), "listen address")
	dsn := flag.String("db", envOr("PACKRAT_DATABASE_URL", "postgres:///packrat?host=/var/run/postgresql"), "PostgreSQL connection URL")
	staticDir := flag.String("static", envOr("PACKRAT_STATIC_DIR", ""), "serve the UI from this directory instead of the copy built into the binary (for development)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("packrat %s (%s, build %s)\n", Version, VersionDate, build)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, *dsn, "")
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		log.Fatalf("migrate database: %v", err)
	}
	for _, name := range applied {
		log.Printf("applied schema migration %s", name)
	}

	var static fs.FS
	if *staticDir != "" {
		static = os.DirFS(*staticDir)
		log.Printf("serving the UI from %s", *staticDir)
	} else if static, err = fs.Sub(staticFS, "static"); err != nil {
		log.Fatal(err)
	}

	srv := &api.Server{
		St:   store.New(pool),
		Info: api.Info{Version: Version, Date: VersionDate, Build: build},
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	mux.Handle("/", noCache(http.FileServer(http.FS(static))))

	httpSrv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("packrat %s (%s) listening on %s", Version, build, *addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// noCache prevents browsers holding stale copies of the embedded assets
// across upgrades of the binary.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}
