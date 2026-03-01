package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppiankov/logspectre/internal/commands"
)

func executeCommand(args ...string) (string, string, error) {
	cmd := commands.NewRootCmd("1.2.3", "abc1234", "2025-01-15T00:00:00Z")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// --- Version command ---

func TestVersionOutput(t *testing.T) {
	out, _, err := executeCommand("version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "1.2.3") {
		t.Errorf("expected version in output, got: %s", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("expected commit in output, got: %s", out)
	}
	if !strings.Contains(out, "2025-01-15T00:00:00Z") {
		t.Errorf("expected date in output, got: %s", out)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	_, _, err := executeCommand("version", "extra")
	if err == nil {
		t.Fatal("expected error for extra args, got nil")
	}
}

// --- Scan command ---

func TestScanHelp(t *testing.T) {
	out, _, err := executeCommand("scan", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, flag := range []string{"--platform", "--format", "--region", "--idle-days", "--min-cost"} {
		if !strings.Contains(out, flag) {
			t.Errorf("expected %q in help output, got: %s", flag, out)
		}
	}
}

func TestScanDefaultFlags(t *testing.T) {
	out, _, err := executeCommand("scan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not yet implemented") {
		t.Errorf("expected 'not yet implemented' in output, got: %s", out)
	}
}

func TestScanValidPlatforms(t *testing.T) {
	for _, p := range []string{"aws", "gcp", "azure", "all"} {
		_, _, err := executeCommand("scan", "--platform", p)
		if err != nil {
			t.Errorf("platform %q should be valid, got error: %v", p, err)
		}
	}
}

func TestScanInvalidPlatform(t *testing.T) {
	_, _, err := executeCommand("scan", "--platform", "digitalocean")
	if err == nil {
		t.Fatal("expected error for invalid platform")
	}
	if !strings.Contains(err.Error(), "invalid platform") {
		t.Errorf("expected 'invalid platform' in error, got: %v", err)
	}
}

func TestScanInvalidFormat(t *testing.T) {
	_, _, err := executeCommand("scan", "--format", "xml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected 'invalid format' in error, got: %v", err)
	}
}

func TestScanInvalidIdleDays(t *testing.T) {
	_, _, err := executeCommand("scan", "--idle-days", "0")
	if err == nil {
		t.Fatal("expected error for idle-days=0")
	}
	if !strings.Contains(err.Error(), "idle-days must be at least 1") {
		t.Errorf("expected idle-days error message, got: %v", err)
	}
}

func TestScanInvalidMinCost(t *testing.T) {
	_, _, err := executeCommand("scan", "--min-cost", "-5")
	if err == nil {
		t.Fatal("expected error for negative min-cost")
	}
	if !strings.Contains(err.Error(), "min-cost must be non-negative") {
		t.Errorf("expected min-cost error message, got: %v", err)
	}
}

// --- Init command ---

func TestInitCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	out, _, err := executeCommand("init")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, ".logspectre.yaml")
	if !strings.Contains(out, path) {
		t.Errorf("expected path in output, got: %s", out)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content := string(data)

	for _, expected := range []string{"platforms:", "idle_days:", "format:", "exclude:"} {
		if !strings.Contains(content, expected) {
			t.Errorf("expected %q in config template", expected)
		}
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.WriteFile(filepath.Join(dir, ".logspectre.yaml"), []byte("existing"), 0644); err != nil {
		t.Fatalf("create existing config: %v", err)
	}

	_, _, err = executeCommand("init")
	if err == nil {
		t.Fatal("expected error when config already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

// --- Root command ---

func TestRootNoArgsShowsHelp(t *testing.T) {
	out, _, err := executeCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "logspectre") {
		t.Errorf("expected 'logspectre' in root help output, got: %s", out)
	}
}
