// Command warren is the Warren server binary. One static binary serves the
// HTMX UI and the REST API; replicas are stateless and identical.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/saku75/warren/internal/auth"
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
	case "user":
		err = userCmd(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "warren: unknown command %q\n\nusage: warren [serve|migrate|user|version]\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "warren:", err)
		os.Exit(1)
	}
}

// userCmd manages user accounts from the CLI; `user create` is the
// bootstrap path for the first administrator. The password is read from
// stdin so it never appears in argv or the environment.
func userCmd(args []string) error {
	if len(args) < 1 || args[0] != "create" {
		return fmt.Errorf("usage: warren user create -username <u> [-name <display>] [-email <e>] [-admin] (password on stdin)")
	}

	fs := flag.NewFlagSet("user create", flag.ContinueOnError)
	username := fs.String("username", "", "username (required)")
	name := fs.String("name", "", "display name")
	email := fs.String("email", "", "email address")
	admin := fs.Bool("admin", false, "grant administrator access")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *username == "" {
		return fmt.Errorf("user create: -username is required")
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("user create: reading password from stdin: %w", scanner.Err())
	}
	password := strings.TrimSpace(scanner.Text())

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

	u, err := auth.NewService(pool).CreateLocalUser(ctx, auth.UserInput{
		Username:    *username,
		DisplayName: *name,
		Email:       *email,
		Password:    password,
		IsAdmin:     *admin,
	})
	if err != nil {
		return err
	}
	log.Info("user created", "username", u.Username, "id", u.ID, "admin", u.IsAdmin)
	return nil
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
