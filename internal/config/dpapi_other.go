//go:build !windows

package config

// secretPrefix marks API keys encrypted with Windows DPAPI. It is only
// produced on Windows; other platforms keep plaintext so development and CI
// environments behave like the pre-encryption builds.
const secretPrefix = "enc:v1:"

// encryptSecret is a no-op on non-Windows platforms: there is no DPAPI, and
// the desktop target is Windows-only.
func encryptSecret(plain string) (string, error) {
	return plain, nil
}

// decryptSecret is a no-op on non-Windows platforms.
func decryptSecret(encoded string) (string, error) {
	return encoded, nil
}
