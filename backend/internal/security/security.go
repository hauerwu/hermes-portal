// Package security: scrypt password hashing (same scheme as Hermes
// Agent's dashboard basic-auth provider), JWT sessions and API keys.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/scrypt"
	"hermesportal/internal/config"
)

// scrypt parameters — identical envelope to hermes-agent's
// plugins/dashboard_auth/basic (scrypt$n$r$p$<salt_b64>$<dk_b64>).
const (
	scryptN         = 1 << 14
	scryptR         = 8
	scryptP         = 1
	scryptDKLen     = 32
	scryptSaltBytes = 16
)

// HashPassword hashes a plaintext password with scrypt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, scryptSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := scryptKey([]byte(password), salt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("scrypt$%d$%d$%d$%s$%s",
		scryptN, scryptR, scryptP,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(dk)), nil
}

// VerifyPassword checks a plaintext password against a stored hash.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false
	}
	n, err1 := strconv.Atoi(parts[1])
	r, err2 := strconv.Atoi(parts[2])
	p, err3 := strconv.Atoi(parts[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	salt, err4 := base64.StdEncoding.DecodeString(parts[4])
	expected, err5 := base64.StdEncoding.DecodeString(parts[5])
	if err4 != nil || err5 != nil {
		return false
	}
	dk, err := scryptKey([]byte(password), salt, n, r, p, len(expected))
	if err != nil {
		return false
	}
	return hmac.Equal(dk, expected)
}

func scryptKey(password, salt []byte, params ...int) ([]byte, error) {
	n, r, p, dkLen := scryptN, scryptR, scryptP, scryptDKLen
	if len(params) == 4 {
		n, r, p, dkLen = params[0], params[1], params[2], params[3]
	}
	return scrypt.Key(password, salt, n, r, p, dkLen)
}

// ── JWT ────────────────────────────────────────────────────────────────

// Claims is the portal JWT payload.
type Claims struct {
	TenantID *uint  `json:"tid,omitempty"`
	Role     string `json:"role"`
	Type     string `json:"typ"` // access | refresh
	jwt.RegisteredClaims
}

// MakeAccessToken mints an access or refresh token.
func MakeAccessToken(cfg *config.Config, userID uint, tenantID *uint, role, tokenType string) (string, error) {
	ttl := time.Duration(cfg.JWTAccessTTL) * time.Second
	if tokenType == "refresh" {
		ttl = time.Duration(cfg.JWTRefreshTTL) * time.Second
	}
	claims := Claims{
		TenantID: tenantID,
		Role:     role,
		Type:     tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
}

// ParseToken validates a JWT and returns its claims.
func ParseToken(cfg *config.Config, token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ── API keys ───────────────────────────────────────────────────────────

// APIKeyPrefix marks portal-issued gateway keys.
const APIKeyPrefix = "hp_"

// GenerateAPIKey returns a random plaintext key; only the hash is stored.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return APIKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashAPIKey returns the SHA-256 hex digest of a plaintext key.
func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// KeyPrefix returns the first 8 chars of a key for UI display.
func KeyPrefix(plain string) string {
	if len(plain) > 8 {
		return plain[:8]
	}
	return plain
}

// RandomSecret returns a URL-safe random string (instance secrets).
func RandomSecret(nbytes int) (string, error) {
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// JSON helpers for config blobs.
func MarshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func UnmarshalJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}
