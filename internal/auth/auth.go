package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const RefreshCookieName = "qs_refresh"

type TokenMaker struct {
	secret []byte
	issuer string
}

func NewTokenMaker(jwtSecret string) (*TokenMaker, error) {
	if len(jwtSecret) < 32 {
		return nil, errors.New("JWT_SECRET too short (use 32+ chars)")
	}
	return &TokenMaker{
		secret: []byte(jwtSecret),
		issuer: "guess-who-game",
	}, nil
}

type AccessClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"uid"`
}

func (tm *TokenMaker) NewAccessToken(userID string, ttl time.Duration) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)

	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		UserID: userID,
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(tm.secret)
	return s, exp, err
}

func (tm *TokenMaker) ParseAccessToken(token string) (*AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		return tm.secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

// Refresh tokens are random opaque strings (NOT JWT), stored server-side by hash.
func NewRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
