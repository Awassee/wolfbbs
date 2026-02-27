package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/mods"
	"wolfbbs/internal/network"
	"wolfbbs/internal/repository"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("oputil", flag.ContinueOnError)
	global.SetOutput(stderr)
	db := global.String("db", "", "database DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	if err := global.Parse(args); err != nil {
		return 2
	}
	rest := global.Args()
	if len(rest) == 0 {
		printUsage(stdout)
		return 1
	}

	dbURL := strings.TrimSpace(*db)
	if dbURL == "" {
		dbURL = repository.ResolveDatabaseURL()
	}

	switch strings.ToLower(strings.TrimSpace(rest[0])) {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "status":
		return cmdStatus(dbURL, stdout, stderr)
	case "users":
		return cmdUsers(dbURL, rest[1:], stdout, stderr)
	case "boards":
		return cmdBoards(dbURL, rest[1:], stdout, stderr)
	case "network":
		return cmdNetwork(dbURL, rest[1:], stdout, stderr)
	case "mods":
		return cmdMods(rest[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", rest[0])
		printUsage(stderr)
		return 2
	}
}

func cmdStatus(dbURL string, stdout, stderr io.Writer) int {
	storage, err := repository.OpenStorageFromEnv(dbURL)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open storage: %v\n", err)
		return 1
	}
	defer storage.Close()

	users, _ := storage.Users.List()
	boards, _ := storage.Boards.List()
	msgCount := 0
	for _, board := range boards {
		msgs, listErr := storage.Messages.ListByBoard(board.ID)
		if listErr == nil {
			msgCount += len(msgs)
		}
	}

	_, _ = fmt.Fprintf(stdout, "users=%d boards=%d messages=%d\n", len(users), len(boards), msgCount)
	return 0
}

func cmdUsers(dbURL string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "users command requires a subcommand: list | set-role")
		return 2
	}
	storage, err := repository.OpenStorageFromEnv(dbURL)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open storage: %v\n", err)
		return 1
	}
	defer storage.Close()
	authSvc := auth.NewService(storage.Users)

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		users, listErr := authSvc.ListUsers()
		if listErr != nil {
			_, _ = fmt.Fprintf(stderr, "list users: %v\n", listErr)
			return 1
		}
		for _, user := range users {
			last := "-"
			if user.LastLoginAt != nil {
				last = user.LastLoginAt.UTC().Format("2006-01-02 15:04:05")
			}
			_, _ = fmt.Fprintf(stdout, "%d\t%s\t%s\tenabled=%t\tbanned=%t\tverified=%t\tlast=%s\n",
				user.ID, user.Handle, user.Role, user.Enabled, user.Banned, user.Verified, last)
		}
		return 0
	case "set-role":
		fs := flag.NewFlagSet("oputil users set-role", flag.ContinueOnError)
		fs.SetOutput(stderr)
		handle := fs.String("handle", "", "target handle")
		role := fs.String("role", "", "role (user|moderator|sysop)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*handle) == "" || strings.TrimSpace(*role) == "" {
			_, _ = fmt.Fprintln(stderr, "set-role requires --handle and --role")
			return 2
		}
		if err := authSvc.SetRole(*handle, *role); err != nil {
			_, _ = fmt.Fprintf(stderr, "set role: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "updated role for %s to %s\n", strings.TrimSpace(*handle), strings.TrimSpace(*role))
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown users subcommand: %s\n", args[0])
		return 2
	}
}

func cmdBoards(dbURL string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "boards command requires a subcommand: list | create | delete")
		return 2
	}
	storage, err := repository.OpenStorageFromEnv(dbURL)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open storage: %v\n", err)
		return 1
	}
	defer storage.Close()

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		boards, listErr := storage.Boards.List()
		if listErr != nil {
			_, _ = fmt.Fprintf(stderr, "list boards: %v\n", listErr)
			return 1
		}
		for _, board := range boards {
			_, _ = fmt.Fprintf(stdout, "%d\t%s\t%s\n", board.ID, board.Name, board.Description)
		}
		return 0
	case "create":
		fs := flag.NewFlagSet("oputil boards create", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "board name")
		description := fs.String("description", "", "board description")
		createdBy := fs.Int64("created-by", 0, "created by user id")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*name) == "" {
			_, _ = fmt.Fprintln(stderr, "create requires --name")
			return 2
		}
		board := &domain.Board{
			Name:        strings.TrimSpace(*name),
			Description: strings.TrimSpace(*description),
			CreatedBy:   *createdBy,
		}
		if err := storage.Boards.Create(board); err != nil {
			_, _ = fmt.Fprintf(stderr, "create board: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "created board id=%d name=%s\n", board.ID, board.Name)
		return 0
	case "delete":
		fs := flag.NewFlagSet("oputil boards delete", flag.ContinueOnError)
		fs.SetOutput(stderr)
		idRaw := fs.String("id", "", "board id")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		id, parseErr := strconv.ParseInt(strings.TrimSpace(*idRaw), 10, 64)
		if parseErr != nil || id <= 0 {
			_, _ = fmt.Fprintln(stderr, "delete requires a valid --id")
			return 2
		}
		if err := storage.Boards.Delete(id); err != nil {
			_, _ = fmt.Fprintf(stderr, "delete board: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "deleted board id=%d\n", id)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown boards subcommand: %s\n", args[0])
		return 2
	}
}

func cmdNetwork(dbURL string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "network command requires a subcommand: status | export | import | queue-netmail | import-queue")
		return 2
	}
	storage, err := repository.OpenStorageFromEnv(dbURL)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "open storage: %v\n", err)
		return 1
	}
	defer storage.Close()

	spoolDir := strings.TrimSpace(os.Getenv("WOLFBBS_NET_SPOOL_DIR"))
	if spoolDir == "" {
		spoolDir = ".wolfbbs/network"
	}
	svc := network.NewService(spoolDir, storage.Boards, storage.Messages, storage.Users, storage.Mail)
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		status, err := svc.Status()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "network status: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "spool=%s inbound=%d outbound=%d\n", status.SpoolDir, status.InboundPackets, status.OutboundPackets)
		return 0
	case "export":
		fs := flag.NewFlagSet("oputil network export", flag.ContinueOnError)
		fs.SetOutput(stderr)
		format := fs.String("format", network.FormatQWK, "format (ftn|bso|qwk)")
		boardID := fs.Int64("board", 0, "source board id")
		out := fs.String("out", "", "output packet path (optional)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *boardID <= 0 {
			_, _ = fmt.Fprintln(stderr, "export requires --board")
			return 2
		}
		path, err := svc.ExportBoard(*format, *boardID)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "export packet: %v\n", err)
			return 1
		}
		if strings.TrimSpace(*out) != "" {
			if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
				_, _ = fmt.Fprintf(stderr, "create output dir: %v\n", err)
				return 1
			}
			if err := os.Rename(path, *out); err != nil {
				_, _ = fmt.Fprintf(stderr, "move packet to --out path: %v\n", err)
				return 1
			}
			path = *out
		}
		_, _ = fmt.Fprintf(stdout, "exported %s\n", path)
		return 0
	case "import":
		fs := flag.NewFlagSet("oputil network import", flag.ContinueOnError)
		fs.SetOutput(stderr)
		in := fs.String("in", "", "input packet path")
		boardID := fs.Int64("board", 0, "default board id")
		authorID := fs.Int64("author", 0, "default author id")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*in) == "" {
			_, _ = fmt.Fprintln(stderr, "import requires --in")
			return 2
		}
		count, err := svc.ImportPacket(*in, *boardID, *authorID)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "import packet: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "imported=%d\n", count)
		return 0
	case "queue-netmail":
		fs := flag.NewFlagSet("oputil network queue-netmail", flag.ContinueOnError)
		fs.SetOutput(stderr)
		from := fs.Int64("from", 0, "from user id")
		to := fs.String("to", "", "recipient handle")
		subject := fs.String("subject", "", "subject")
		body := fs.String("body", "", "body")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path, err := svc.QueueNetmail(*from, *to, *subject, *body)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "queue netmail: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "queued %s\n", path)
		return 0
	case "import-queue":
		fs := flag.NewFlagSet("oputil network import-queue", flag.ContinueOnError)
		fs.SetOutput(stderr)
		boardID := fs.Int64("board", 0, "default board id for non-netmail packets")
		authorID := fs.Int64("author", 0, "default author id for non-netmail packets")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		count, err := svc.ImportInboundQueue(*boardID, *authorID)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "import queue: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "imported=%d\n", count)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown network subcommand: %s\n", args[0])
		return 2
	}
}

func cmdMods(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "mods command requires a subcommand: list")
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list":
		manager := mods.NewManager(time.Hour)
		one := mods.NewOneLinerzMod(40)
		rumorz := mods.NewRumorzMod(nil)
		bbsList := mods.NewBBSListMod(100)
		who := mods.NewWhoOnlineMod(func() int { return 0 })
		_ = manager.Register(one, true)
		_ = manager.Register(rumorz, true)
		_ = manager.Register(bbsList, true)
		_ = manager.Register(who, true)
		for _, row := range manager.Snapshot() {
			_, _ = fmt.Fprintf(stdout, "%s\tenabled=%t\tdescription=%s\n", row.ID, row.Enabled, row.Description)
		}
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown mods subcommand: %s\n", args[0])
		return 2
	}
}

func printUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, `WolfBBS oputil

Usage:
  oputil [--db <dsn>] status
  oputil [--db <dsn>] users list
  oputil [--db <dsn>] users set-role --handle <name> --role <user|moderator|sysop>
  oputil [--db <dsn>] boards list
  oputil [--db <dsn>] boards create --name <title> [--description <text>] [--created-by <uid>]
  oputil [--db <dsn>] boards delete --id <id>
  oputil [--db <dsn>] network status
  oputil [--db <dsn>] network export --format <ftn|bso|qwk> --board <id> [--out <path>]
  oputil [--db <dsn>] network import --in <packet.json> [--board <id>] [--author <id>]
  oputil [--db <dsn>] network queue-netmail --from <uid> --to <handle> --subject <s> --body <b>
  oputil [--db <dsn>] network import-queue [--board <id>] [--author <id>]
  oputil mods list`)
}
