package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const writerFixture = `# top comment
modules:
  chatflow_enabled: no   # inline comment kept
  server_enabled: no
overlay:
  enable: yes
  alerts:
    discord: no
twitch:
  client_secret: "${TWITCH_SECRET}"
`

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "chatbridge.settings")
	if err := os.WriteFile(p, []byte(writerFixture), 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return p
}

func TestSetModuleEnabled_FlipsFlagAndPreserves(t *testing.T) {
	p := writeFixture(t)

	if err := SetModuleEnabled(p, "ChatFlow", true); err != nil {
		t.Fatalf("SetModuleEnabled failed: %v", err)
	}

	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, "chatflow_enabled: yes") {
		t.Errorf("flag not flipped to yes:\n%s", got)
	}
	if !strings.Contains(got, "server_enabled: no") {
		t.Errorf("unrelated flag was altered:\n%s", got)
	}
	if !strings.Contains(got, "# top comment") || !strings.Contains(got, "inline comment kept") {
		t.Errorf("comments not preserved:\n%s", got)
	}
	if !strings.Contains(got, "${TWITCH_SECRET}") {
		t.Errorf("unexpanded env reference not preserved:\n%s", got)
	}
	// The value must stay an unquoted plain scalar (YesNoBool rejects quotes).
	if strings.Contains(got, "chatflow_enabled: \"yes\"") {
		t.Errorf("flag was quoted, which YesNoBool would reject:\n%s", got)
	}
}

func TestSetBoolByPath_Submodule(t *testing.T) {
	p := writeFixture(t)

	if err := SetBoolByPath(p, []string{"overlay", "alerts", "discord"}, true); err != nil {
		t.Fatalf("SetBoolByPath failed: %v", err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	if !strings.Contains(string(out), "discord: yes") {
		t.Errorf("nested submodule flag not set:\n%s", string(out))
	}
}

func TestSetBoolByPath_MissingKey(t *testing.T) {
	p := writeFixture(t)

	if err := SetBoolByPath(p, []string{"overlay", "nope"}, true); err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestSetModuleEnabled_UnknownModule(t *testing.T) {
	p := writeFixture(t)

	if err := SetModuleEnabled(p, "DoesNotExist", true); err == nil {
		t.Fatal("expected error for unknown module, got nil")
	}
}
