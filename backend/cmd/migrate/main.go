// Command migrate é uma CLI fina sobre golang-migrate.
//
// Uso:
//
//	migrate up                # aplica todas as migrations pendentes
//	migrate down 1            # reverte 1 migration
//	migrate version           # mostra a versão atual
//	migrate force <versão>    # marca a versão (use após corrigir uma migration suja)
//
// Lê DATABASE_URL via internal/config. Migrations residem em ./migrations,
// configurável via MIGRATIONS_PATH.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL é obrigatório")
		os.Exit(1)
	}

	path := os.Getenv("MIGRATIONS_PATH")
	if path == "" {
		path = "./migrations"
	}

	m, err := migrate.New("file://"+path, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate init: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			fmt.Fprintf(os.Stderr, "migrate close: src=%v db=%v\n", srcErr, dbErr)
		}
	}()

	cmd := os.Args[1]
	switch cmd {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fmt.Fprintf(os.Stderr, "up: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("ok: migrations aplicadas")

	case "down":
		n := 1
		if len(os.Args) >= 3 {
			parsed, err := strconv.Atoi(os.Args[2])
			if err != nil || parsed <= 0 {
				fmt.Fprintln(os.Stderr, "down: argumento deve ser inteiro positivo")
				os.Exit(2)
			}
			n = parsed
		}
		if err := m.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			fmt.Fprintf(os.Stderr, "down: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ok: %d migration(s) revertida(s)\n", n)

	case "version":
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("nenhuma migration aplicada")
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "version: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("versão=%d dirty=%v\n", v, dirty)

	case "force":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "force: informe a versão alvo")
			os.Exit(2)
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "force: versão inválida: %v\n", err)
			os.Exit(2)
		}
		if err := m.Force(v); err != nil {
			fmt.Fprintf(os.Stderr, "force: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ok: versão forçada para %d\n", v)

	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `uso: migrate <comando> [args]
comandos:
  up                  aplica todas as migrations pendentes
  down [N]            reverte N migrations (default 1)
  version             mostra a versão atual e flag dirty
  force <versão>      força a versão registrada (uso em recovery)`)
}
