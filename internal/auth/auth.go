package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func ValidateRegistration(name string, email string, password string) string {
	if len([]rune(name)) < 2 || len([]rune(name)) > 80 {
		return "Имя должно быть от 2 до 80 символов."
	}
	if !emailPattern.MatchString(email) {
		return "Введите корректный email."
	}
	if len([]rune(password)) < 12 {
		return "Пароль должен быть не короче 12 символов."
	}
	localPart, _, _ := strings.Cut(email, "@")
	if localPart != "" && strings.Contains(strings.ToLower(password), strings.ToLower(localPart)) {
		return "Пароль не должен содержать часть email."
	}
	return ""
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func VerifyPassword(password string, storedHash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) == nil
}

func NewToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func SecureEqual(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
	window  time.Duration
}

func NewRateLimiter(window time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string][]time.Time),
		window:  window,
	}
}

func (limiter *RateLimiter) Allow(key string, limit int) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-limiter.window)
	events := limiter.buckets[key]
	kept := events[:0]

	for _, event := range events {
		if event.After(cutoff) {
			kept = append(kept, event)
		}
	}

	if len(kept) >= limit {
		limiter.buckets[key] = kept
		return false
	}

	kept = append(kept, now)
	limiter.buckets[key] = kept
	return true
}
