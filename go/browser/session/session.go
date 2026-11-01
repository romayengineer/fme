package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/mxschmitt/playwright-go"
)

const sessionFilePath = ".session.json"

// Store abstracts session persistence so auth flows can be tested with a fake.
type Store interface {
	Load() (*playwright.StorageState, error)
	Save(*playwright.StorageState) error
	Delete() error
}

// FileStore stores the session in .session.json in the working directory.
type FileStore struct{}

func (FileStore) Load() (*playwright.StorageState, error)   { return LoadSession() }
func (FileStore) Save(state *playwright.StorageState) error { return SaveSession(state) }
func (FileStore) Delete() error                             { return DeleteSession() }

// SaveSession stores the browser cookies and storage state. The file contains
// live Facebook session credentials, so it is readable only by the current user.
func SaveSession(storageState *playwright.StorageState) error {
	if storageState == nil {
		return errors.New("storage state is nil")
	}

	data, err := json.MarshalIndent(storageState, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal storage state: %w", err)
	}

	if err := os.WriteFile(sessionFilePath, data, 0o600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// LoadSession returns nil and no error when no session file exists. A file that
// cannot be read or parsed is reported so the caller can discard it and log in.
func LoadSession() (*playwright.StorageState, error) {
	data, err := os.ReadFile(sessionFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}

	var storageState playwright.StorageState
	if err := json.Unmarshal(data, &storageState); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}

	return &storageState, nil
}

// DeleteSession removes the stored session. A missing file is not an error.
func DeleteSession() error {
	err := os.Remove(sessionFilePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
