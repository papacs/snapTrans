// Package jsonfile replaces local JSON files without truncating the live copy.
package jsonfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrCorrupt = errors.New("invalid JSON file")

func Read(path string) ([]byte, error) {
	return ReadValidated(path, nil)
}

// ReadValidated also rejects values that parse as JSON but do not match the
// caller's schema. The validator runs before either copy is restored.
func ReadValidated(path string, validate func([]byte) error) ([]byte, error) {
	raw, err := readValid(path, validate)
	if err == nil || (!errors.Is(err, ErrCorrupt) && !errors.Is(err, os.ErrNotExist)) {
		return raw, err
	}
	backup, backupErr := readValid(path+".bak", validate)
	if backupErr != nil {
		if !errors.Is(backupErr, os.ErrNotExist) && (!errors.Is(backupErr, ErrCorrupt) || errors.Is(err, os.ErrNotExist)) {
			return nil, backupErr
		}
		return nil, err
	}
	if err := replace(path, backup); err != nil {
		return nil, fmt.Errorf("restore JSON backup: %w", err)
	}
	return backup, nil
}

func readValid(path string, validate func([]byte) error) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil && !json.Valid(raw) {
		err = fmt.Errorf("%w: %s", ErrCorrupt, path)
	}
	if err == nil && validate != nil {
		if validationErr := validate(raw); validationErr != nil {
			err = fmt.Errorf("%w: %s: %v", ErrCorrupt, path, validationErr)
		}
	}
	return raw, err
}

func Write(path string, raw []byte) error {
	if !json.Valid(raw) {
		return ErrCorrupt
	}
	previous, err := readValid(path, nil)
	if err == nil {
		if err := replace(path+".bak", previous); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrCorrupt) {
		return err
	}
	return replace(path, raw)
}

// Reset replaces both copies with the same sanitized value. Explicit deletions
// must not leave the removed data available for later backup recovery.
func Reset(path string, raw []byte) error {
	if !json.Valid(raw) {
		return ErrCorrupt
	}
	if err := replace(path+".bak", raw); err != nil {
		return err
	}
	return replace(path, raw)
}

func replace(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".snaptrans-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replaceFile(file.Name(), path)
}
