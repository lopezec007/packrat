// Command packrat serves the Packrat packing-list web application.
//
// Usage:
//
//	packrat [-addr :8082] [-db postgres:///packrat?host=/var/run/postgresql] [-backup-dir backups] [-static DIR]
//	packrat -version
//
// The flags default to the environment variables PACKRAT_ADDR,
// PACKRAT_DATABASE_URL, PACKRAT_BACKUP_DIR and PACKRAT_STATIC_DIR.
//
// On start the schema is brought up to date from the migrations embedded in
// the binary (internal/db/migrations). Before that changes a database that
// is already in use, the database is dumped into the backup directory
// (internal/db/backup.go); if the dump cannot be made, nothing is migrated
// and the service does not start. -no-backup (PACKRAT_NO_BACKUP=1) skips the
// dump, for when a backup has been taken some other way.
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
	Version     = "0.2.0"
	VersionDate = "2026-11-24"
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
	backupDir := flag.String("backup-dir", envOr("PACKRAT_BACKUP_DIR", "backups"), "directory the database is dumped into before its schema is upgraded")
	noBackup := flag.Bool("no-backup", envOr("PACKRAT_NO_BACKUP", "") != "", "upgrade the schema without dumping the database first (only when it has been backed up some other way)")
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

	// A database that is in use is backed up before its schema is changed.
	applied, err := db.MigrateWith(ctx, pool, func(ctx context.Context, from, to int) error {
		if *noBackup {
			log.Printf("upgrading the schema from version %d to %d WITHOUT a backup (-no-backup)", from, to)
			return nil
		}
		path, err := db.Backup(ctx, *dsn, *backupDir, fmt.Sprintf("schema-%d-to-%d", from, to))
		if err != nil {
			return fmt.Errorf("the database could not be backed up, so it has not been touched: %w", err)
		}
		log.Printf("backed up the database to %s before upgrading its schema from version %d to %d", path, from, to)
		return nil
	})
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
