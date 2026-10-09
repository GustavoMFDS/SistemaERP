package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                               string
	ServiceName                       string
	AppVersion                        string
	HTTPAddr                          string
	DatabaseURL                       string
	RedisURL                          string
	RedisAddr                         string
	RedisPassword                     string
	RedisDB                           int
	JWTSecret                         string
	JWTIssuer                         string
	AccessTokenTTL                    time.Duration
	RefreshTokenTTL                   time.Duration
	OTelEnabled                       bool
	OTelExporter                      string
	OTelOTLPEndpoint                  string
	LogLevel                          string
	AllowNegativeStock                bool
	CORSAllowedOrigins                []string
	CORSAllowedMethods                []string
	CORSAllowedHeaders                []string
	TrustedProxyCIDRs                 []string
	MetricsBearerToken                string
	MetricsBasicUser                  string
	MetricsBasicPass                  string
	RateLimitLogin                    int
	RateLimitLoginID                  int
	RateLimitLoginIPID                int
	RateLimitRefresh                  int
	RateLimitLogout                   int
	RateLimitSales                    int
	RateLimitFiscal                   int
	FiscalProvider                    string
	NFCeCertificateSecretDir          string
	NFCeSchemaDir                     string
	NFCeSchemaEntrypoint              string
	NFCeEventSchemaEntrypoint         string
	NFCeInutilizationSchemaEntrypoint string
	NFCeSEFAZHomologationEnabled      bool
	NFCeSEFAZProductionEnabled        bool
	DisableRedis                      bool
	PrivacyContactEmail               string
	AppPublicURL                      string
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
		Env:                               env,
		ServiceName:                       getEnv("SERVICE_NAME", "sistemaemgo-api"),
		AppVersion:                        getEnv("APP_VERSION", "dev"),
		HTTPAddr:                          getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:                       dbURL,
		RedisURL:                          strings.TrimSpace(os.Getenv("REDIS_URL")),
		RedisAddr:                         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:                     redisPassword,
		RedisDB:                           getEnvInt("REDIS_DB", 0),
		JWTSecret:                         jwtSecret,
		JWTIssuer:                         getEnv("JWT_ISSUER", "sistemaemgo"),
		AccessTokenTTL:                    time.Duration(getEnvInt("ACCESS_TOKEN_TTL_MINUTES", 15)) * time.Minute,
		RefreshTokenTTL:                   time.Duration(getEnvInt("REFRESH_TOKEN_TTL_MINUTES", 43200)) * time.Minute,
		OTelEnabled:                       getEnvBool("OTEL_ENABLED", !strings.EqualFold(env, "prod") && !strings.EqualFold(env, "production")),
		OTelExporter:                      strings.ToLower(strings.TrimSpace(getEnv("OTEL_EXPORTER", "stdout"))),
		OTelOTLPEndpoint:                  strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		LogLevel:                          getEnv("LOG_LEVEL", "info"),
		AllowNegativeStock:                getEnvBool("ALLOW_NEGATIVE_STOCK", false),
		CORSAllowedOrigins:                getEnvList("CORS_ALLOWED_ORIGINS", defaultCORSOrigins(env)),
		CORSAllowedMethods:                getEnvList("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
		CORSAllowedHeaders:                getEnvList("CORS_ALLOWED_HEADERS", []string{"Authorization", "Content-Type", "Accept", "Idempotency-Key"}),
		TrustedProxyCIDRs:                 getEnvList("TRUSTED_PROXY_CIDRS", nil),
		MetricsBearerToken:                strings.TrimSpace(os.Getenv("METRICS_BEARER_TOKEN")),
		MetricsBasicUser:                  strings.TrimSpace(os.Getenv("METRICS_BASIC_USER")),
		MetricsBasicPass:                  strings.TrimSpace(os.Getenv("METRICS_BASIC_PASS")),
		RateLimitLogin:                    getEnvInt("RATE_LIMIT_LOGIN_PER_MINUTE", 10),
		RateLimitLoginID:                  getEnvInt("RATE_LIMIT_LOGIN_IDENTIFIER_PER_MINUTE", 5),
		RateLimitLoginIPID:                getEnvInt("RATE_LIMIT_LOGIN_IP_IDENTIFIER_PER_MINUTE", 5),
		RateLimitRefresh:                  getEnvInt("RATE_LIMIT_REFRESH_PER_MINUTE", 30),
		RateLimitLogout:                   getEnvInt("RATE_LIMIT_LOGOUT_PER_MINUTE", 30),
		RateLimitSales:                    getEnvInt("RATE_LIMIT_SALES_PER_MINUTE", 60),
		RateLimitFiscal:                   getEnvInt("RATE_LIMIT_FISCAL_PER_MINUTE", 20),
		FiscalProvider:                    strings.ToLower(strings.TrimSpace(getEnv("FISCAL_PROVIDER", "mvp"))),
		NFCeCertificateSecretDir:          strings.TrimSpace(os.Getenv("NFCE_CERTIFICATE_SECRET_DIR")),
		NFCeSchemaDir:                     strings.TrimSpace(os.Getenv("NFCE_SCHEMA_DIR")),
		NFCeSchemaEntrypoint:              strings.TrimSpace(os.Getenv("NFCE_SCHEMA_ENTRYPOINT")),
		NFCeEventSchemaEntrypoint:         strings.TrimSpace(os.Getenv("NFCE_EVENT_SCHEMA_ENTRYPOINT")),
		NFCeInutilizationSchemaEntrypoint: strings.TrimSpace(os.Getenv("NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT")),
		NFCeSEFAZHomologationEnabled:      getEnvBool("NFCE_SEFAZ_HOMOLOGATION_ENABLED", false),
		NFCeSEFAZProductionEnabled:        getEnvBool("NFCE_SEFAZ_PRODUCTION_ENABLED", false),
		DisableRedis:                      getEnvBool("DISABLE_REDIS", false),
		PrivacyContactEmail:               strings.TrimSpace(os.Getenv("PRIVACY_CONTACT_EMAIL")),
		AppPublicURL:                      strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")),
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

	if !c.DisableRedis && strings.TrimSpace(c.RedisURL) == "" && strings.TrimSpace(c.RedisAddr) == "" {
		errs = append(errs, "REDIS_URL or REDIS_ADDR is required")
	}
	if c.IsProdLike() && c.DisableRedis {
		errs = append(errs, "DISABLE_REDIS must not be enabled in staging/prod")
	}
	if strings.TrimSpace(c.RedisURL) != "" {
		if err := validateRedisURL(c.RedisURL, c.IsProdLike()); err != nil {
			errs = append(errs, err.Error())
		}
	} else if c.IsProdLike() {
		errs = append(errs, "REDIS_URL with rediss:// is required in staging/prod")
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
		if containsPlaceholder(ls) {
			errs = append(errs, "JWT_SECRET must not be a placeholder")
		}
	}

	if c.AccessTokenTTL <= 0 {
		errs = append(errs, "ACCESS_TOKEN_TTL_MINUTES must be > 0")
	}
	if c.IsProdLike() && c.AccessTokenTTL > 15*time.Minute {
		errs = append(errs, "ACCESS_TOKEN_TTL_MINUTES must be <= 15 in staging/prod")
	}
	if c.RefreshTokenTTL <= 0 {
		errs = append(errs, "REFRESH_TOKEN_TTL_MINUTES must be > 0")
	}
	if strings.TrimSpace(c.JWTIssuer) == "" {
		errs = append(errs, "JWT_ISSUER is required")
	}

	if c.OTelEnabled {
		if strings.TrimSpace(c.ServiceName) == "" {
			errs = append(errs, "SERVICE_NAME is required when OTEL_ENABLED=true")
		}
		switch strings.ToLower(strings.TrimSpace(c.OTelExporter)) {
		case "stdout", "otlp":
		default:
			errs = append(errs, "OTEL_EXPORTER must be one of stdout|otlp")
		}
		if strings.EqualFold(c.OTelExporter, "otlp") && strings.TrimSpace(c.OTelOTLPEndpoint) == "" {
			errs = append(errs, "OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_EXPORTER=otlp")
		}
	}
	if c.IsProdLike() && len(c.CORSAllowedOrigins) == 0 {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must be explicit in staging/prod")
	}
	if err := validateCORSOrigins(c.CORSAllowedOrigins, c.IsProdLike()); err != nil {
		errs = append(errs, err.Error())
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			errs = append(errs, fmt.Sprintf("TRUSTED_PROXY_CIDRS contains invalid CIDR %q", cidr))
		}
	}
	if c.IsProdLike() && c.MetricsBearerToken == "" && (c.MetricsBasicUser == "" || c.MetricsBasicPass == "") {
		errs = append(errs, "METRICS_BEARER_TOKEN or METRICS_BASIC_USER/METRICS_BASIC_PASS is required in staging/prod")
	}
	if c.IsProdLike() && getEnvBool("ALLOW_DEMO_SEED", false) {
		errs = append(errs, "ALLOW_DEMO_SEED must not be enabled in staging/prod")
	}
	if c.RateLimitLogin < 0 || c.RateLimitLoginID < 0 || c.RateLimitLoginIPID < 0 || c.RateLimitRefresh < 0 || c.RateLimitLogout < 0 || c.RateLimitSales < 0 || c.RateLimitFiscal < 0 {
		errs = append(errs, "rate limit values must be >= 0")
	}

	fiscalProvider := strings.ToLower(strings.TrimSpace(c.FiscalProvider))
	if fiscalProvider == "" {
		fiscalProvider = "mvp"
	}
	switch fiscalProvider {
	case "mvp", "disabled", "sefaz":
	default:
		errs = append(errs, "FISCAL_PROVIDER must be one of mvp|disabled|sefaz")
	}
	if c.IsProdLike() && fiscalProvider == "mvp" {
		errs = append(errs, "FISCAL_PROVIDER=mvp is not allowed in staging/prod because it is not SEFAZ-ready; use disabled until a production fiscal provider is configured")
	}

	if (c.NFCeSchemaDir == "") != (c.NFCeSchemaEntrypoint == "") {
		errs = append(errs, "NFCE_SCHEMA_DIR and NFCE_SCHEMA_ENTRYPOINT must be configured together")
	}

	if c.NFCeEventSchemaEntrypoint != "" && c.NFCeSchemaDir == "" {
		errs = append(errs, "NFCE_SCHEMA_DIR is required when NFCE_EVENT_SCHEMA_ENTRYPOINT is configured")
	}
	if c.NFCeInutilizationSchemaEntrypoint != "" && c.NFCeSchemaDir == "" {
		errs = append(errs, "NFCE_SCHEMA_DIR is required when NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT is configured")
	}

	if c.NFCeSEFAZHomologationEnabled && c.NFCeSEFAZProductionEnabled {
		errs = append(errs, "SEFAZ homologation and production transmission flags are mutually exclusive")
	}
	if c.NFCeSEFAZHomologationEnabled {
		if e == "prod" || e == "production" {
			errs = append(errs, "NFCE_SEFAZ_HOMOLOGATION_ENABLED must not be enabled in production")
		}
		if fiscalProvider != "sefaz" {
			errs = append(errs, "FISCAL_PROVIDER=sefaz is required when SEFAZ homologation is enabled")
		}
		if strings.TrimSpace(c.NFCeCertificateSecretDir) == "" {
			errs = append(errs, "NFCE_CERTIFICATE_SECRET_DIR is required when SEFAZ homologation is enabled")
		}
		if c.NFCeSchemaDir == "" || c.NFCeSchemaEntrypoint == "" {
			errs = append(errs, "NFCE schema bundle is required when SEFAZ homologation is enabled")
		}
		if c.NFCeEventSchemaEntrypoint == "" {
			errs = append(errs, "NFCE_EVENT_SCHEMA_ENTRYPOINT is required when SEFAZ homologation is enabled")
		}
		if c.NFCeInutilizationSchemaEntrypoint == "" {
			errs = append(errs, "NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT is required when SEFAZ homologation is enabled")
		}
	}
	if c.NFCeSEFAZProductionEnabled {
		if e != "prod" && e != "production" {
			errs = append(errs, "NFCE_SEFAZ_PRODUCTION_ENABLED is only allowed in production")
		}
		if fiscalProvider != "sefaz" {
			errs = append(errs, "FISCAL_PROVIDER=sefaz is required when SEFAZ production is enabled")
		}
		if strings.TrimSpace(c.NFCeCertificateSecretDir) == "" {
			errs = append(errs, "NFCE_CERTIFICATE_SECRET_DIR is required when SEFAZ production is enabled")
		}
		if c.NFCeSchemaDir == "" || c.NFCeSchemaEntrypoint == "" {
			errs = append(errs, "NFCE schema bundle is required when SEFAZ production is enabled")
		}
		if c.NFCeEventSchemaEntrypoint == "" {
			errs = append(errs, "NFCE_EVENT_SCHEMA_ENTRYPOINT is required when SEFAZ production is enabled")
		}
		if c.NFCeInutilizationSchemaEntrypoint == "" {
			errs = append(errs, "NFCE_INUTILIZATION_SCHEMA_ENTRYPOINT is required when SEFAZ production is enabled")
		}
	}
	if fiscalProvider == "sefaz" &&
		!c.NFCeSEFAZHomologationEnabled &&
		!c.NFCeSEFAZProductionEnabled {
		errs = append(errs, "FISCAL_PROVIDER=sefaz requires an explicit SEFAZ transmission environment flag")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func validateCORSOrigins(origins []string, prodLike bool) error {
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			return errors.New("CORS_ALLOWED_ORIGINS must not contain wildcard '*'")
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS contains invalid origin %q", origin)
		}
		if prodLike && (strings.HasPrefix(u.Host, "localhost") || strings.HasPrefix(u.Host, "127.0.0.1")) {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must not use localhost origins in staging/prod: %q", origin)
		}
	}
	return nil
}

func defaultCORSOrigins(env string) []string {
	e := strings.ToLower(strings.TrimSpace(env))
	if e == "prod" || e == "production" || e == "staging" {
		return nil
	}
	return []string{"http://localhost:5173", "http://127.0.0.1:5173"}
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
	if prodLike && (weak[strings.ToLower(pass)] || containsPlaceholder(pass)) {
		return errors.New("DATABASE_URL password is too weak for staging/prod")
	}
	if prodLike {
		if strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode"))) != "verify-full" {
			return errors.New("DATABASE_URL must use sslmode=verify-full in staging/prod")
		}
	}
	return nil
}

func validateRedisURL(raw string, prodLike bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("REDIS_URL is invalid: %w", err)
	}
	switch u.Scheme {
	case "redis", "rediss":
	default:
		return errors.New("REDIS_URL scheme must be redis or rediss")
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("REDIS_URL must include host")
	}
	if prodLike {
		if u.Scheme != "rediss" {
			return errors.New("REDIS_URL must use rediss:// in staging/prod")
		}
		pass, hasPass := u.User.Password()
		if !hasPass || strings.TrimSpace(pass) == "" || containsPlaceholder(pass) {
			return errors.New("REDIS_URL must include a non-placeholder password in staging/prod")
		}
	}
	return nil
}

func containsPlaceholder(v string) bool {
	normalized := strings.ToLower(strings.TrimSpace(v))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return strings.Contains(normalized, "change_me") ||
		strings.Contains(normalized, "changeme") ||
		strings.Contains(normalized, "replace_with") ||
		strings.Contains(normalized, "required") ||
		strings.Contains(normalized, "placeholder")
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

func getEnvList(key string, def []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
