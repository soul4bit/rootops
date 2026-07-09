package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Addr         string
	CookieSecure bool
	DBPath       string
	PublicURL    string
	ProjectRoot  string
	SessionTTL   time.Duration
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	VerifyTTL    time.Duration
}

func Load() Config {
	root := getenv("ROOTOPS_ROOT", "")
	if root == "" {
		workingDir, err := os.Getwd()
		if err == nil {
			root = workingDir
		}
	}
	loadDotEnv(filepath.Join(root, ".env"))

	host := getenv("ROOTOPS_HOST", "127.0.0.1")
	port := getenv("ROOTOPS_PORT", "8080")
	addr := getenv("ROOTOPS_ADDR", net.JoinHostPort(host, port))

	dataDir := getenv("ROOTOPS_DATA_DIR", filepath.Join(root, "data"))
	dbPath := getenv("ROOTOPS_DB", filepath.Join(dataDir, "rootops.sqlite3"))
	publicURL := strings.TrimRight(getenv("ROOTOPS_PUBLIC_URL", ""), "/")
	secure := strings.EqualFold(os.Getenv("ROOTOPS_COOKIE_SECURE"), "true") || os.Getenv("ROOTOPS_COOKIE_SECURE") == "1"

	smtpUsername := getenv("ROOTOPS_SMTP_USERNAME", "")
	smtpFrom := getenv("ROOTOPS_SMTP_FROM", "")
	if smtpFrom == "" && strings.Contains(smtpUsername, "@") {
		smtpFrom = smtpUsername
	}

	return Config{
		Addr:         addr,
		CookieSecure: secure,
		DBPath:       dbPath,
		PublicURL:    publicURL,
		ProjectRoot:  root,
		SMTPHost:     getenv("ROOTOPS_SMTP_HOST", ""),
		SMTPPort:     getenv("ROOTOPS_SMTP_PORT", "2525"),
		SMTPUsername: smtpUsername,
		SMTPPassword: getenv("ROOTOPS_SMTP_PASSWORD", ""),
		SMTPFrom:     smtpFrom,
		SessionTTL:   7 * 24 * time.Hour,
		VerifyTTL:    24 * time.Hour,
	}
}

func getenv(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func loadDotEnv(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}

		_ = os.Setenv(key, value)
	}
}
