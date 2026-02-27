package repository

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_SQLITE_PATH")); v != "" {
		return "sqlite://" + v
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
	Boards   BoardRepository
	Messages MessageRepository
	Mail     PrivateMailRepository
	Admin    AdminRepository
	Doors    DoorRepository
	Resets   PasswordResetRepository
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
			Boards:   NewInMemoryBoardRepository(),
			Messages: NewInMemoryMessageRepository(),
			Mail:     NewInMemoryPrivateMailRepository(),
			Admin:    NewInMemoryAdminRepository(),
			Doors:    NewInMemoryDoorRepository(),
			Resets:   NewInMemoryPasswordResetRepository(),
			Close:    func() {},
		}, nil
	}
	if isSQLiteDSN(dsn) {
		db, err := OpenSQLite(dsn)
		if err != nil {
			return nil, err
		}
		return &Storage{
			Users:    NewSQLiteUserRepository(db),
			Boards:   NewSQLiteBoardRepository(db),
			Messages: NewSQLiteMessageRepository(db),
			Mail:     NewSQLitePrivateMailRepository(db),
			Admin:    NewSQLiteAdminRepository(db),
			Doors:    NewInMemoryDoorRepository(),
			Resets:   NewSQLitePasswordResetRepository(db),
			Close: func() {
				_ = db.Close()
			},
		}, nil
	}
	db, err := OpenPostgres(dsn)
	if err != nil {
		return nil, err
	}
	return &Storage{
		Users:    NewPostgresUserRepository(db),
		Boards:   NewPostgresBoardRepository(db),
		Messages: NewPostgresMessageRepository(db),
		Mail:     NewPostgresPrivateMailRepository(db),
		Admin:    NewPostgresAdminRepository(db),
		Doors:    NewPostgresDoorRepository(db),
		Resets:   NewPostgresPasswordResetRepository(db),
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
	if err := lockPostgresMigrations(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(915207314))
	}()
	if err := runMigrations(context.Background(), db, migrationFS); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func lockPostgresMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, int64(915207314))
	return err
}

func OpenSQLite(dsn string) (*sql.DB, error) {
	sqliteDSN := normalizeSQLiteDSN(dsn)
	if sqliteDSN == "" {
		return nil, errors.New("sqlite dsn is required")
	}
	if sqliteDSN != ":memory:" && !strings.HasPrefix(sqliteDSN, "file:") {
		if err := os.MkdirAll(filepath.Dir(sqliteDSN), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", sqliteDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureSQLiteSchema(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func isSQLiteDSN(dsn string) bool {
	dsn = strings.ToLower(strings.TrimSpace(dsn))
	return strings.HasPrefix(dsn, "sqlite://") || strings.HasPrefix(dsn, "file:") || dsn == ":memory:"
}

func normalizeSQLiteDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return ""
	}
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	if strings.HasPrefix(strings.ToLower(dsn), "sqlite://") {
		path := strings.TrimPrefix(dsn, "sqlite://")
		path = strings.TrimSpace(path)
		if path == "" {
			return ""
		}
		return path
	}
	return dsn
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
