package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const tokenBytes = 32

// LoadOrCreateToken returns the daemon auth token, generating a random one on
// first run. The token file is owner-read-only.
func LoadOrCreateToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("token file %s is empty; delete it to regenerate", path)
		}
		return token, nil
	case errors.Is(err, fs.ErrNotExist):
		return createToken(path)
	default:
		return "", fmt.Errorf("read token file: %w", err)
	}
}

func createToken(path string) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}
	return token, nil
}
