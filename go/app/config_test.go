package app

import (
	"os"
	"testing"
)

// unsetEnv removes variables for the test and restores them afterwards.
// t.Setenv would leave an empty value, which godotenv does not override.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		old, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		key := key
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func TestLoadConfigWithPersistentProfile(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, "FME_USER_DATA_DIR", "FME_USERNAME", "FME_PASSWORD", "FME_BROWSER_CHANNEL", "FME_PROFILE_DIRECTORY")

	env := "FME_USER_DATA_DIR=/tmp/fme-profile\nFME_PROFILE_DIRECTORY=Profile 1\n"
	if err := os.WriteFile(".env", []byte(env), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.UserDataDir != "/tmp/fme-profile" {
		t.Errorf("UserDataDir = %q", config.UserDataDir)
	}
	if config.BrowserChannel != "chrome" {
		t.Errorf("BrowserChannel = %q, want chrome", config.BrowserChannel)
	}
	if config.ProfileDirectory != "Profile 1" {
		t.Errorf("ProfileDirectory = %q", config.ProfileDirectory)
	}
}

func TestLoadConfigWithCredentials(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, "FME_USER_DATA_DIR", "FME_USERNAME", "FME_PASSWORD", "FME_BROWSER_CHANNEL", "FME_PROFILE_DIRECTORY")

	env := "FME_USERNAME=user@example.com\nFME_PASSWORD=secret\n"
	if err := os.WriteFile(".env", []byte(env), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.UserCredentials.Username != "user@example.com" {
		t.Errorf("Username = %q", config.UserCredentials.Username)
	}
	if config.UserCredentials.Password != "secret" {
		t.Errorf("Password = %q", config.UserCredentials.Password)
	}
}

func TestLoadConfigRequiresCredentialsOrProfile(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, "FME_USER_DATA_DIR", "FME_USERNAME", "FME_PASSWORD")

	if err := os.WriteFile(".env", nil, 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig returned no error without credentials or a profile")
	}
}
