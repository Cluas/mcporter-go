package mcporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunListReadsConfig(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{
  "mcpServers": {
    "ctx": {
      "description": "Docs",
      "baseUrl": "https://mcp.context7.com/mcp"
    },
    "local": {
      "command": "npx",
      "args": ["-y", "my-server"]
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"list", configPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "ctx\thttp\thttps://mcp.context7.com/mcp\tDocs") {
		t.Fatalf("expected ctx row in output, got %q", out)
	}
	if !strings.Contains(out, "local\tstdio\tnpx") {
		t.Fatalf("expected local row in output, got %q", out)
	}
}

func TestRunCallPlansFromSeparateServerAndTool(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{
  "mcpServers": {
    "ctx": {
      "baseUrl": "https://mcp.context7.com/mcp"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "ctx", "resolve-library-id", "libraryName=react"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "planned call ctx.resolve-library-id via http https://mcp.context7.com/mcp args=libraryName=react") {
		t.Fatalf("unexpected call plan output: %q", stdout.String())
	}
}

func TestRunCallAcceptsDottedTarget(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{
  "mcpServers": {
    "ctx": {
      "baseUrl": "https://mcp.context7.com/mcp"
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "ctx.resolve-library-id"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "planned call ctx.resolve-library-id via http https://mcp.context7.com/mcp") {
		t.Fatalf("unexpected call plan output: %q", stdout.String())
	}
}

func TestRunCallReturnsUnknownServer(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{"mcpServers":{"ctx":{"baseUrl":"https://mcp.context7.com/mcp"}}}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "missing", "resolve"}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stderr.String(), `unknown server "missing"`) {
		t.Fatalf("expected unknown server message, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "available: ctx") {
		t.Fatalf("expected available server list in error, got %q", stderr.String())
	}
}
