package mcporter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRunCallExecutesHTTPTool(t *testing.T) {
	var capturedMethod string
	var capturedPath string
	var capturedBody callRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "jsonrpc":"2.0",
  "id":1,
  "result":{"content":[{"type":"text","text":"resolved /react"}]}
}`))
	}))
	defer server.Close()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{"mcpServers":{"ctx":{"baseUrl":"` + server.URL + `/mcp"}}}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "ctx", "resolve-library-id", "libraryName=react", "count=2", "active=true"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "resolved /react") {
		t.Fatalf("unexpected call output: %q", stdout.String())
	}
	if capturedMethod != http.MethodPost {
		t.Fatalf("expected POST request, got %q", capturedMethod)
	}
	if capturedPath != "/mcp" {
		t.Fatalf("expected /mcp path, got %q", capturedPath)
	}
	if capturedBody.Method != "tools/call" {
		t.Fatalf("expected tools/call method, got %q", capturedBody.Method)
	}
	if capturedBody.Params.Name != "resolve-library-id" {
		t.Fatalf("expected tool name resolve-library-id, got %q", capturedBody.Params.Name)
	}
	if capturedBody.Params.Arguments["libraryName"] != "react" {
		t.Fatalf("expected string arg coercion, got %#v", capturedBody.Params.Arguments["libraryName"])
	}
	if capturedBody.Params.Arguments["count"] != float64(2) {
		t.Fatalf("expected numeric arg coercion, got %#v", capturedBody.Params.Arguments["count"])
	}
	if capturedBody.Params.Arguments["active"] != true {
		t.Fatalf("expected bool arg coercion, got %#v", capturedBody.Params.Arguments["active"])
	}
}

func TestRunCallAcceptsDottedTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	defer server.Close()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{"mcpServers":{"ctx":{"baseUrl":"` + server.URL + `"}}}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "ctx.resolve-library-id"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("unexpected call output: %q", stdout.String())
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

func TestRunCallReturnsStdioNotImplemented(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "mcporter.json")
	content := `{"mcpServers":{"local":{"command":"npx","args":["-y","local-mcp"]}}}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Run([]string{"--config", configPath, "call", "local", "ping"}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stderr.String(), "uses stdio and call is not implemented yet in Go") {
		t.Fatalf("expected stdio message, got %q", stderr.String())
	}
}
