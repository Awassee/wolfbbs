package repository

import (
	"context"
	"database/sql"
	"embed"
	_ "github.com/lib/pq"
	"os"
	"strconv"
	"sort"
	"strings"
	"time"
)

const (
	defaultPostgresHost = "postgres"
	defaultPostgresPort = "5432"
	defaultPingTries    = 10
	defaultPingDelay    = 1 * time.Second
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func ResolveDatabaseURL() string {
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_DATABASE_URL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	host := strings.TrimSpace(os.Getenv("PGHOST"))
	user := strings.TrimSpace(os.Getenv("PGUSER"))
	password := strings.TrimSpace(os.Getenv("PGPASSWORD"))
	db := strings.TrimSpace(os.Getenv("PGDATABASE"))
	port := strings.TrimSpace(os.Getenv("PGPORT"))
	sslMode := strings.TrimSpace(os.Getenv("PGSSLMODE"))
	if host == "" && user == "" && password == "" && db == "" && port == "" {
		return ""
	}
	if host == "" {
		host = defaultPostgresHost
	}
	if port == "" {
		port = defaultPostgresPort
	}
	if db == "" {
		db = "wolfbbs"
	}
	if user == "" {
		user = "wolfbbs"
	}
	if sslMode == "" {
		sslMode = "disable"
	}
	if password != "" {
		return "postgres://" + user + ":" + password + "@" + host + ":" + port + "/" + db + "?sslmode=" + sslMode
	}
	return "postgres://" + user + "@" + host + ":" + port + "/" + db + "?sslmode=" + sslMode
}

type Storage struct {
	Users    UserRepository
	Messages MessageRepository
	Close    func()
}

func OpenStorageFromEnv(override string) (*Storage, error) {
	dsn := strings.TrimSpace(override)
	if dsn == "" {
		dsn = ResolveDatabaseURL()
	}
	if dsn == "" {
		return &Storage{
			Users:    NewInMemoryUserRepository(),
			Messages: NewInMemoryMessageRepository(),
			Close:    func() {},
		}, nil
	}
	db, err := OpenPostgres(dsn)
	if err != nil {
		return nil, err
	}
	return &Storage{
		Users:    NewPostgresUserRepository(db),
		Messages: NewPostgresMessageRepository(db),
		Close: func() {
			_ = db.Close()
		},
	}, nil
}

func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	maxConnEnv := strings.TrimSpace(os.Getenv("WOLFBBS_DB_MAX_CONN"))
	maxConn, _ := strconv.Atoi(maxConnEnv)
	if maxConn <= 0 {
		maxConn = 8
	}
	db.SetMaxOpenConns(maxConn)
	db.SetMaxIdleConns(4)
	pingTries := defaultPingTries
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DB_CONNECT_RETRIES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			pingTries = v
		}
	}
	pingDelay := defaultPingDelay
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DB_CONNECT_DELAY_MS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			pingDelay = time.Duration(v) * time.Millisecond
		}
	}
	var pingErr error
	for i := 0; i < pingTries; i++ {
		pingErr = db.Ping()
		if pingErr == nil {
			break
		}
		if i+1 < pingTries {
			time.Sleep(pingDelay)
		}
	}
	if pingErr != nil {
		_ = db.Close()
		return nil, pingErr
	}
	if err := runMigrations(context.Background(), db, migrationFS); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func runMigrations(ctx context.Context, db *sql.DB, fs embed.FS) error {
	entries, err := fs.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		path := "migrations/" + name
		body, err := fs.ReadFile(path)
		if err != nil {
			return err
		}
		if err := execSQLBatch(ctx, db, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func execSQLBatch(ctx context.Context, db *sql.DB, batch string) error {
	parts := strings.Split(batch, ";")
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
