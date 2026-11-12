package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/cybirdsecurity/myfi-os/internal/auth"
	"github.com/cybirdsecurity/myfi-os/internal/db"
)

// runResetPassword implements `myfi reset-password`: it sets a new password
// for the instance's account, clears any login lockout and signs out every
// session. It is the only way to recover a forgotten password, so it runs on
// the server, where only the operator can reach it.
func runResetPassword(args []string) int {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fromStdin := fs.Bool("password-stdin", false, "read the new password from standard input instead of prompting")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: myfi reset-password [--password-stdin]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set")
		return 1
	}

	password, err := readNewPassword(*fromStdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "password must be at least 8 characters")
		return 1
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		return 1
	}

	sqlxDB, err := db.New(databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to db: %v\n", err)
		return 1
	}
	defer sqlxDB.Close()

	email, err := db.NewUserStore(sqlxDB).ResetPassword(context.Background(), hash)
	if errors.Is(err, sql.ErrNoRows) {
		fmt.Fprintln(os.Stderr, "no account exists yet; open MyFi in a browser to create one")
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Password reset for %s. All sessions were signed out.\n", email)
	return 0
}

// readNewPassword reads the first line of stdin when fromStdin is set, and
// otherwise prompts twice on the terminal without echoing.
func readNewPassword(fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("no terminal to prompt on: run with `docker compose exec -it`, or pass --password-stdin")
	}
	fmt.Fprint(os.Stderr, "New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm new password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}
