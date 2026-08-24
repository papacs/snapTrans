//go:build windows

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPAPISecretRoundTrip(t *testing.T) {
	plain := "sk-0123456789abcdef-secret"
	encrypted, err := encryptSecret(plain)
	require.NoError(t, err)
	require.NotEqual(t, plain, encrypted)
	require.True(t, strings.HasPrefix(encrypted, secretPrefix))

	decrypted, err := decryptSecret(encrypted)
	require.NoError(t, err)
	require.Equal(t, plain, decrypted)
}

func TestDPAPIEmptySecretStaysEmpty(t *testing.T) {
	encrypted, err := encryptSecret("")
	require.NoError(t, err)
	require.Equal(t, "", encrypted)
}

func TestDPAPIDecryptRejectsNonPrefixedValue(t *testing.T) {
	_, err := decryptSecret("plaintext-key")
	require.Error(t, err)
}

func TestSaveWritesEncryptedAPIKeyNotPlaintext(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.APIKey = "sk-plaintext-must-not-leak"
	require.NoError(t, store.Save(expected))

	raw, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(raw), secretPrefix))
	require.False(t, strings.Contains(string(raw), "sk-plaintext-must-not-leak"))
}

func TestSaveAndLoadRoundTripEncryptsAPIKey(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.APIKey = "sk-roundtrip-secret"
	require.NoError(t, store.Save(expected))

	actual, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "sk-roundtrip-secret", actual.APIKey)
}

func TestLoadAcceptsLegacyPlaintextAPIKey(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"apiKey":"legacy-plain-key"}`), 0o600))

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()
	require.NoError(t, err)
	require.Equal(t, "legacy-plain-key", cfg.APIKey)
}
