//go:build windows

package config

import (
	"encoding/base64"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// secretPrefix marks API keys encrypted with Windows DPAPI (CryptProtectData).
// Values without this prefix are treated as legacy plaintext.
const secretPrefix = "enc:v1:"

// encryptSecret protects a plaintext secret with the current Windows user's
// DPAPI key, so the config file no longer stores API keys in cleartext.
func encryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}

	data := []byte(plain)
	input := windows.DataBlob{
		Size: uint32(len(data)),
		Data: &data[0],
	}
	var output windows.DataBlob
	if err := windows.CryptProtectData(
		&input, nil, nil, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &output,
	); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))

	encoded := base64.StdEncoding.EncodeToString(unsafe.Slice(output.Data, int(output.Size)))
	return secretPrefix + encoded, nil
}

// decryptSecret restores a secret produced by encryptSecret. Values that are
// not DPAPI-protected (legacy plaintext configs) are rejected by callers.
func decryptSecret(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, secretPrefix) {
		return "", errors.New("not an encrypted secret")
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, secretPrefix))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}

	input := windows.DataBlob{
		Size: uint32(len(raw)),
		Data: &raw[0],
	}
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(
		&input, nil, nil, 0, nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN, &output,
	); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))

	return string(unsafe.Slice(output.Data, int(output.Size))), nil
}
