package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                string
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	JWTSecret          string
	JWTIssuer          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	LogLevel           string
	AllowNegativeStock bool
}

// LoadFromEnv reads configuration only from process env.
// It does NOT load .env files; use your process manager / Docker / CI to inject vars.
//
// Secrets can be provided either via value (e.g. JWT_SECRET) or via file path
// (e.g. JWT_SECRET_FILE), which is compatible with Docker/K8s secrets.
func LoadFromEnv() (Config, error) {
	env := getEnv("APP_ENV", "dev")
	jwtSecret, err := getEnvOrFile("JWT_SECRET", "JWT_SECRET_FILE")
	if err != nil {
		return Config{}, err
	}
	redisPassword, err := getEnvOrFile("REDIS_PASSWORD", "REDIS_PASSWORD_FILE")
	if err != nil {
		return Config{}, err
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		dbURL, err = buildDatabaseURLFromParts()
		if err != nil {
			return Config{}, err
		}
	}

	cfg := Config{
		Env:                env,
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:        dbURL,
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      redisPassword,
		RedisDB:            getEnvInt("REDIS_DB", 0),
		JWTSecret:          jwtSecret,
		JWTIssuer:          getEnv("JWT_ISSUER", "sistemaemgo"),
		AccessTokenTTL:     time.Duration(getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 30)) * time.Minute,
		RefreshTokenTTL:    time.Duration(getEnvInt("REFRESH_TOKEN_TTL_MINUTES", 43200)) * time.Minute,
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		AllowNegativeStock: getEnvBool("ALLOW_NEGATIVE_STOCK", false),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) IsProdLike() bool {
	e := strings.ToLower(strings.TrimSpace(c.Env))
	return e == "prod" || e == "production" || e == "staging"
}

func (c Config) Validate() error {
	var errs []string

	e := strings.ToLower(strings.TrimSpace(c.Env))
	switch e {
	case "dev", "development", "test", "staging", "prod", "production":
	default:
		errs = append(errs, "APP_ENV must be one of dev|test|staging|prod")
	}

	if strings.TrimSpace(c.DatabaseURL) == "" {
		errs = append(errs, "DATABASE_URL is required")
	} else {
		if err := validateDatabaseURL(c.DatabaseURL, c.IsProdLike()); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if strings.TrimSpace(c.RedisAddr) == "" {
		errs = append(errs, "REDIS_ADDR is required")
	}

	sec := strings.TrimSpace(c.JWTSecret)
	if sec == "" {
		errs = append(errs, "JWT_SECRET is required")
	} else {
		// HS256 minimum: require at least 32 bytes to reduce brute-force risk.
		if len(sec) < 32 {
			errs = append(errs, "JWT_SECRET must be at least 32 characters")
		}
		ls := strings.ToLower(sec)
		if strings.Contains(ls, "change-me") || strings.Contains(ls, "changeme") {
			errs = append(errs, "JWT_SECRET must not be a placeholder")
		}
	}

	if c.AccessTokenTTL <= 0 {
		errs = append(errs, "ACCESS_TOKEN_TTL_MINUTES must be > 0")
	}
	if c.RefreshTokenTTL <= 0 {
		errs = append(errs, "REFRESH_TOKEN_TTL_MINUTES must be > 0")
	}
	if strings.TrimSpace(c.JWTIssuer) == "" {
		errs = append(errs, "JWT_ISSUER is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func NewLogger(cfg Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}

func getEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getEnvOrFile(key, fileKey string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v, nil
	}
	path := strings.TrimSpace(os.Getenv(fileKey))
	if path == "" {
		return "", nil
	}
	// Basic hardening: disallow directories and normalize path.
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("%s: cannot stat secret file: %w", fileKey, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s: secret file path is a directory", fileKey)
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		return "", fmt.Errorf("%s: cannot read secret file: %w", fileKey, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func buildDatabaseURLFromParts() (string, error) {
	// Accept DATABASE_URL first; if absent, build from parts.
	host := strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
	port := strings.TrimSpace(os.Getenv("POSTGRES_PORT"))
	db := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
	user := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	pass, err := getEnvOrFile("DB_PASSWORD", "DB_PASSWORD_FILE")
	if err != nil {
		return "", err
	}
	if host == "" || port == "" || db == "" || user == "" {
		return "", errors.New("DATABASE_URL is required (or set POSTGRES_HOST/PORT/DB/USER and DB_PASSWORD)")
	}
	if pass == "" {
		return "", errors.New("DB_PASSWORD is required when building DATABASE_URL")
	}
	u := &url.URL{Scheme: "postgres", Host: fmt.Sprintf("%s:%s", host, port), Path: db}
	u.User = url.UserPassword(user, pass)
	q := u.Query()
	q.Set("sslmode", getEnv("PGSSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func validateDatabaseURL(raw string, prodLike bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("DATABASE_URL is invalid: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New("DATABASE_URL scheme must be postgres")
	}
	if u.User == nil {
		return errors.New("DATABASE_URL must include username and password")
	}
	pass, hasPass := u.User.Password()
	if !hasPass || strings.TrimSpace(pass) == "" {
		return errors.New("DATABASE_URL must include password")
	}
	weak := map[string]bool{"postgres": true, "password": true, "admin": true, "123": true, "sistemaemgo": true}
	if prodLike && weak[strings.ToLower(pass)] {
		return errors.New("DATABASE_URL password is too weak for staging/prod")
	}
	return nil
}

func getEnvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
