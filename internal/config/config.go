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
	ProjectRoot  string
	SessionTTL   time.Duration
}

func Load() Config {
	root := getenv("ROOTOPS_ROOT", "")
	if root == "" {
		workingDir, err := os.Getwd()
		if err == nil {
			root = workingDir
		}
	}

	host := getenv("ROOTOPS_HOST", "127.0.0.1")
	port := getenv("ROOTOPS_PORT", "8080")
	addr := getenv("ROOTOPS_ADDR", net.JoinHostPort(host, port))

	dataDir := getenv("ROOTOPS_DATA_DIR", filepath.Join(root, "data"))
	dbPath := getenv("ROOTOPS_DB", filepath.Join(dataDir, "rootops.sqlite3"))
	secure := strings.EqualFold(os.Getenv("ROOTOPS_COOKIE_SECURE"), "true") || os.Getenv("ROOTOPS_COOKIE_SECURE") == "1"

	return Config{
		Addr:         addr,
		CookieSecure: secure,
		DBPath:       dbPath,
		ProjectRoot:  root,
		SessionTTL:   7 * 24 * time.Hour,
	}
}

func getenv(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
