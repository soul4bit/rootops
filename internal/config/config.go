package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr       string
	ContentDir string
	ProjectDir string
}

func Load() Config {
	projectDir, err := os.Getwd()
	if err != nil {
		projectDir = "."
	}

	host := getenv("ROOTOPS_HOST", "127.0.0.1")
	port := getenv("ROOTOPS_PORT", "8080")

	return Config{
		Addr:       getenv("ROOTOPS_ADDR", net.JoinHostPort(host, port)),
		ContentDir: getenv("ROOTOPS_CONTENT_DIR", filepath.Join(projectDir, "content")),
		ProjectDir: projectDir,
	}
}

func getenv(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
