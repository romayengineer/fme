package session

import (
	"os"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestSaveLoadSessionRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())

	state := &playwright.StorageState{
		Cookies: []playwright.Cookie{
			{Name: "session", Value: "secret", Domain: "facebook.com", Path: "/"},
		},
	}

	if err := SaveSession(state); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	info, err := os.Stat(sessionFilePath)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("session file mode = %o, want 600", got)
	}

	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession returned nil state")
	}
	if len(got.Cookies) != 1 || got.Cookies[0].Value != "secret" {
		t.Fatalf("cookies = %#v, want the saved cookie", got.Cookies)
	}
}

func TestLoadSessionMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadSession = %#v, want nil", got)
	}
}

func TestLoadSessionCorruptFile(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile(sessionFilePath, []byte("not json"), 0600); err != nil {
		t.Fatalf("write corrupt session: %v", err)
	}

	got, err := LoadSession()
	if err == nil {
		t.Fatal("LoadSession returned no error for corrupt JSON")
	}
	if got != nil {
		t.Fatalf("LoadSession = %#v, want nil", got)
	}
}

func TestDeleteSessionMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := DeleteSession(); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
}
