package main

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

type fakes struct {
	dbPassword   string
	appKey       string
	jwtSecret    string
	mailPassword string
	accessKey    string
	secretKey    string
	apiToken     string
}

func newFakes() fakes {
	return fakes{
		dbPassword:   randomString(18),
		appKey:       "base64:" + randomString(43),
		jwtSecret:    randomHex(32),
		mailPassword: randomString(16),
		accessKey:    "AKIA" + strings.ToUpper(randomString(16)),
		secretKey:    randomString(40),
		apiToken:     randomHex(24),
	}
}

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
