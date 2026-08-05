// Command migrate runs goose migrations against the configured Postgres
// instance. Run automatically on ECS deploy, manually in dev:
//
//	docker compose run --rm migrate
//	# or
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down
//	go run ./cmd/migrate status
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate failed:", err)
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	db, err := goose.OpenDBWithDriver("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "migrations"
	}

	goose.SetBaseFS(os.DirFS("."))

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		if err := goose.Up(db, dir); err != nil {
			return fmt.Errorf("goose up: %w", err)
		}
	case "up-by-one":
		if err := goose.UpByOne(db, dir); err != nil {
			return fmt.Errorf("goose up-by-one: %w", err)
		}
	case "down":
		if err := goose.Down(db, dir); err != nil {
			return fmt.Errorf("goose down: %w", err)
		}
	case "redo":
		if err := goose.Redo(db, dir); err != nil {
			return fmt.Errorf("goose redo: %w", err)
		}
	case "reset":
		if err := goose.Reset(db, dir); err != nil {
			return fmt.Errorf("goose reset: %w", err)
		}
	case "status":
		if err := goose.Status(db, dir); err != nil {
			return fmt.Errorf("goose status: %w", err)
		}
	case "version":
		v, err := goose.GetDBVersion(db)
		if err != nil {
			return fmt.Errorf("goose version: %w", err)
		}
		fmt.Println("version:", v)
	case "create":
		if len(os.Args) < 3 {
			return errors.New("usage: migrate create <name>")
		}
		migrationType := "go"
		if len(os.Args) > 3 {
			migrationType = os.Args[3]
		}
		if err := goose.Create(db, dir, os.Args[2], migrationType); err != nil {
			return fmt.Errorf("goose create: %w", err)
		}
	default:
		return fmt.Errorf("unknown command %q (up, down, redo, reset, status, version, create)", cmd)
	}

	fmt.Println("migrate:", cmd, "ok")
	return nil
}

// avoid unused-import warning when the file is refactored
var _ = strconv.Itoa
