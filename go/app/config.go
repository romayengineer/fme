package app

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/romayengineer/fme/go/browser/facebook"
	"github.com/romayengineer/fme/go/core/logging"
)

// Config is the browser/session configuration read from .env and the process
// environment.
type Config struct {
	UserCredentials  facebook.UserCredentials
	UserDataDir      string
	BrowserChannel   string
	ProfileDirectory string
	// Headless is the FME_HEADLESS request, or nil when it is unset (the
	// display is then probed at launch time). See ResolveHeadless.
	Headless *bool
}

// LoadConfig loads .env (if present) and resolves the browser session settings.
// A persistent Chrome profile is preferred; username/password is the fallback.
func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("load .env file: %w", err)
	}

	config := configFromEnv()
	if config.UserDataDir == "" && (config.UserCredentials.Username == "" || config.UserCredentials.Password == "") {
		return nil, errors.New("set FME_USER_DATA_DIR or FME_USERNAME/FME_PASSWORD in .env")
	}

	config.BrowserChannel = defaultChannel(config.UserDataDir, config.BrowserChannel)
	printConfigSource(config)

	return config, nil
}

// configFromEnv reads every session setting from the environment.
func configFromEnv() *Config {
	return &Config{
		UserCredentials: facebook.UserCredentials{
			Username: os.Getenv("FME_USERNAME"),
			Password: os.Getenv("FME_PASSWORD"),
		},
		UserDataDir:      os.Getenv("FME_USER_DATA_DIR"),
		BrowserChannel:   os.Getenv("FME_BROWSER_CHANNEL"),
		ProfileDirectory: os.Getenv("FME_PROFILE_DIRECTORY"),
		Headless:         headlessFromEnv(),
	}
}

// defaultChannel uses the installed Chrome channel for profile launches.
func defaultChannel(userDataDir, channel string) string {
	if userDataDir != "" && channel == "" {
		return "chrome"
	}
	return channel
}

// printConfigSource reports which login source will be used.
func printConfigSource(config *Config) {
	if config.UserDataDir != "" {
		fmt.Printf("Using browser profile: %s\n", config.UserDataDir)
		return
	}
	fmt.Printf("Loaded credentials - Username: %s\n", config.UserCredentials.Username)
}

// headlessFromEnv reads FME_HEADLESS. An unset or invalid value yields nil so
// the caller can fall back to probing for a display.
func headlessFromEnv() *bool {
	raw, ok := os.LookupEnv("FME_HEADLESS")
	if !ok || raw == "" {
		return nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		logging.LogWarn0("config", "ignoring invalid FME_HEADLESS value", "value", raw)
		return nil
	}
	return &value
}

// ResolveHeadless picks the browser mode for this launch. The first non-nil
// override wins (normally -headless, then FME_HEADLESS); otherwise Chromium
// runs headless when no graphical display is available, because headed Chrome
// exits with "Missing X server or $DISPLAY" in that case.
func ResolveHeadless(overrides ...*bool) bool {
	for _, override := range overrides {
		if override != nil {
			return *override
		}
	}
	return displayForHeadedChrome() == ""
}

// displayForHeadedChrome returns the display a windowed Chromium can use, or ""
// when there is none. On Linux it adopts a running X server when DISPLAY is
// missing, which is common for agent, cron and ssh sessions on a desktop.
func displayForHeadedChrome() string {
	if runtime.GOOS != "linux" {
		return "native"
	}
	if display := os.Getenv("DISPLAY"); display != "" {
		return display
	}
	if display := runningXDisplay(); display != "" {
		if err := os.Setenv("DISPLAY", display); err == nil {
			logging.LogInfo0("config", "using running X display", "display", display)
		}
		return display
	}
	return os.Getenv("WAYLAND_DISPLAY")
}

// runningXDisplay returns ":N" for the lowest responsive X socket, or "".
func runningXDisplay() string {
	for n := range 10 {
		display := fmt.Sprintf(":%d", n)
		if _, err := os.Stat("/tmp/.X11-unix/X" + strconv.Itoa(n)); err == nil {
			return display
		}
	}
	return ""
}
