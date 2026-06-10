// Command warren is the Warren server binary. One static binary serves the
// HTMX UI and the REST API; replicas are stateless and identical.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/db"
	"github.com/saku75/warren/internal/server"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "migrate":
		err = migrate()
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "warren: unknown command %q\n\nusage: warren [serve|migrate|version]\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "warren:", err)
		os.Exit(1)
	}
}

func setup() (*slog.Logger, config.Config, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	cfg, err := config.FromEnv()
	if err != nil {
		return nil, config.Config{}, err
	}
	if cfg.DatabaseURL == "" {
		return nil, config.Config{}, fmt.Errorf("WARREN_DATABASE_URL is required")
	}
	return log, cfg, nil
}

func serve() error {
	log, cfg, err := setup()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		ran, err := db.Migrate(ctx, pool)
		if err != nil {
			return err
		}
		if len(ran) > 0 {
			log.Info("migrations applied", "versions", ran)
		}
	}

	return server.New(cfg, log, version, pool).Run(ctx)
}

func migrate() error {
	log, cfg, err := setup()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	ran, err := db.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	log.Info("migrations complete", "applied", ran, "schema_version", db.LatestVersion())
	return nil
}
