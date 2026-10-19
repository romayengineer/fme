package app

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/romayengineer/fme/go/browser/facebook"
)

// Config is the browser/session configuration read from .env and the process
// environment.
type Config struct {
	UserCredentials  facebook.UserCredentials
	UserDataDir      string
	BrowserChannel   string
	ProfileDirectory string
}

// LoadConfig loads .env (if present) and resolves the browser session settings.
// A persistent Chrome profile is preferred; username/password is the fallback.
func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("load .env file: %w", err)
	}

	username := os.Getenv("FME_USERNAME")
	password := os.Getenv("FME_PASSWORD")
	userDataDir := os.Getenv("FME_USER_DATA_DIR")
	browserChannel := os.Getenv("FME_BROWSER_CHANNEL")
	profileDirectory := os.Getenv("FME_PROFILE_DIRECTORY")

	if userDataDir == "" && (username == "" || password == "") {
		return nil, fmt.Errorf("set FME_USER_DATA_DIR or FME_USERNAME/FME_PASSWORD in .env")
	}

	if userDataDir != "" && browserChannel == "" {
		browserChannel = "chrome"
	}

	if userDataDir != "" {
		fmt.Printf("Using browser profile: %s\n", userDataDir)
	} else {
		fmt.Printf("Loaded credentials - Username: %s\n", username)
	}

	return &Config{
		UserCredentials: facebook.UserCredentials{
			Username: username,
			Password: password,
		},
		UserDataDir:      userDataDir,
		BrowserChannel:   browserChannel,
		ProfileDirectory: profileDirectory,
	}, nil
}
