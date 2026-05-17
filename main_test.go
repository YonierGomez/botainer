package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types"
)

// ─── containerFirstName ───────────────────────────────────────────────────────

func TestContainerFirstName_normal(t *testing.T) {
	c := types.Container{Names: []string{"/my-container"}}
	got := containerFirstName(c)
	if got != "my-container" {
		t.Errorf("expected 'my-container', got %q", got)
	}
}

func TestContainerFirstName_noLeadingSlash(t *testing.T) {
	c := types.Container{Names: []string{"my-container"}}
	got := containerFirstName(c)
	if got != "my-container" {
		t.Errorf("expected 'my-container', got %q", got)
	}
}

func TestContainerFirstName_emptyNames_longID(t *testing.T) {
	c := types.Container{Names: []string{}, ID: "abcdef123456abcdef12"}
	got := containerFirstName(c)
	if got != "abcdef123456" {
		t.Errorf("expected short ID 'abcdef123456', got %q", got)
	}
}

func TestContainerFirstName_emptyNames_shortID(t *testing.T) {
	c := types.Container{Names: []string{}, ID: "abc"}
	got := containerFirstName(c)
	if got != "abc" {
		t.Errorf("expected 'abc', got %q", got)
	}
}

// ─── findComposeFile ──────────────────────────────────────────────────────────

func TestFindComposeFile_composeYaml(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "compose.yaml")
	os.WriteFile(f, []byte("services:\n  web:\n    image: nginx\n"), 0644)
	got := findComposeFile(dir)
	if got != f {
		t.Errorf("expected %q, got %q", f, got)
	}
}

func TestFindComposeFile_dockerComposeYml(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(f, []byte("services:\n  web:\n    image: nginx\n"), 0644)
	got := findComposeFile(dir)
	if got != f {
		t.Errorf("expected %q, got %q", f, got)
	}
}

func TestFindComposeFile_notFound(t *testing.T) {
	dir := t.TempDir()
	got := findComposeFile(dir)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFindComposeFile_priority(t *testing.T) {
	// compose.yaml should take priority over docker-compose.yml
	dir := t.TempDir()
	f1 := filepath.Join(dir, "compose.yaml")
	f2 := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(f1, []byte("services:\n"), 0644)
	os.WriteFile(f2, []byte("services:\n"), 0644)
	got := findComposeFile(dir)
	if got != f1 {
		t.Errorf("expected compose.yaml (%q) to win, got %q", f1, got)
	}
}

// ─── serviceExistsInCompose ───────────────────────────────────────────────────

func writeComposeFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestServiceExistsInCompose_found(t *testing.T) {
	f := writeComposeFile(t, `services:
  nginx:
    image: nginx
  redis:
    image: redis
`)
	if !serviceExistsInCompose(f, "nginx") {
		t.Error("expected to find service 'nginx'")
	}
	if !serviceExistsInCompose(f, "redis") {
		t.Error("expected to find service 'redis'")
	}
}

func TestServiceExistsInCompose_notFound(t *testing.T) {
	f := writeComposeFile(t, `services:
  nginx:
    image: nginx
`)
	if serviceExistsInCompose(f, "postgres") {
		t.Error("should not find service 'postgres'")
	}
}

func TestServiceExistsInCompose_noFile(t *testing.T) {
	if serviceExistsInCompose("/nonexistent/compose.yaml", "web") {
		t.Error("should return false for nonexistent file")
	}
}

func TestServiceExistsInCompose_realWorldCompose(t *testing.T) {
	content := `version: "3.8"
services:
  botainer:
    build: /home/ubuntu/botainer
    restart: unless-stopped
    environment:
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
  postgres:
    image: postgres:alpine
    restart: unless-stopped
`
	f := writeComposeFile(t, content)
	if !serviceExistsInCompose(f, "botainer") {
		t.Error("expected to find service 'botainer'")
	}
	if !serviceExistsInCompose(f, "postgres") {
		t.Error("expected to find service 'postgres'")
	}
	if serviceExistsInCompose(f, "redis") {
		t.Error("should not find service 'redis'")
	}
}
