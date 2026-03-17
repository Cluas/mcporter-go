package mcporter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const Version = "0.1.0-go"

func Run(args []string, stdout, stderr io.Writer) int {
	args, configPath, err := extractConfigPath(args)
	if err != nil {
		fmt.Fprintf(stderr, "mcporter: %v\n", err)
		return 1
	}

	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		printHelp(stdout)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, Version)
		return 0
	case "list":
		path := configPath
		if len(args) > 1 {
			path = args[1]
		}
		if err := listServers(path, stdout); err != nil {
			fmt.Fprintf(stderr, "mcporter: %v\n", err)
			return 1
		}
		return 0
	case "call":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "mcporter: usage: mcporter call <server> <tool> [arg=value...]")
			return 1
		}
		if err := planCall(configPath, args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "mcporter: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "mcporter: unknown command %q\n", args[0])
		printHelp(stderr)
		return 1
	}
}

func defaultConfigPath() string {
	return "config/mcporter.json"
}

type configFile struct {
	MCPServers map[string]serverConfig `json:"mcpServers"`
}

type serverConfig struct {
	Description string       `json:"description"`
	BaseURL     string       `json:"baseUrl"`
	Command     commandValue `json:"command"`
	Args        []string     `json:"args"`
}

type commandValue struct {
	parts []string
}

func (c *commandValue) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		trimmed := strings.TrimSpace(asString)
		if trimmed == "" {
			c.parts = nil
			return nil
		}
		c.parts = []string{trimmed}
		return nil
	}

	var asArray []string
	if err := json.Unmarshal(data, &asArray); err == nil {
		parts := make([]string, 0, len(asArray))
		for _, item := range asArray {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		c.parts = parts
		return nil
	}

	return fmt.Errorf("command must be a string or string array, got: %s", strings.TrimSpace(string(data)))
}

func listServers(path string, out io.Writer) error {
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	if len(cfg.MCPServers) == 0 {
		fmt.Fprintln(out, "No servers configured.")
		return nil
	}

	for _, name := range sortedServerNames(cfg) {
		srv := cfg.MCPServers[name]
		transport := "stdio"
		endpoint := commandSummary(srv)
		if strings.TrimSpace(srv.BaseURL) != "" {
			transport = "http"
			endpoint = srv.BaseURL
		}
		if endpoint == "" {
			endpoint = "(unspecified)"
		}
		if strings.TrimSpace(srv.Description) == "" {
			fmt.Fprintf(out, "%s\t%s\t%s\n", name, transport, endpoint)
			continue
		}
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", name, transport, endpoint, srv.Description)
	}

	return nil
}

func planCall(configPath string, args []string, out io.Writer) error {
	serverName, toolName, toolArgs, err := parseCallInput(args)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	srv, ok := cfg.MCPServers[serverName]
	if !ok {
		return fmt.Errorf("unknown server %q (available: %s)", serverName, strings.Join(sortedServerNames(cfg), ", "))
	}

	if strings.TrimSpace(srv.BaseURL) == "" {
		return fmt.Errorf("server %q uses stdio and call is not implemented yet in Go", serverName)
	}

	arguments, err := parseToolArgs(toolArgs)
	if err != nil {
		return err
	}

	return callToolHTTP(http.DefaultClient, srv.BaseURL, toolName, arguments, out)
}

func parseToolArgs(toolArgs []string) (map[string]any, error) {
	arguments := make(map[string]any, len(toolArgs))
	for _, arg := range toolArgs {
		key, rawValue, ok := strings.Cut(arg, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid tool argument %q (expected key=value)", arg)
		}
		arguments[strings.TrimSpace(key)] = coerceCallArgValue(strings.TrimSpace(rawValue))
	}
	return arguments, nil
}

func coerceCallArgValue(raw string) any {
	lower := strings.ToLower(raw)
	switch lower {
	case "true":
		return true
	case "false":
		return false
	}

	if i, err := strconv.Atoi(raw); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && strings.Contains(raw, ".") {
		return f
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}

	return raw
}

type callRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      int               `json:"id"`
	Method  string            `json:"method"`
	Params  callRequestParams `json:"params"`
}

type callRequestParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type callResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type callResult struct {
	Content []callContent `json:"content"`
}

type callContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func callToolHTTP(client *http.Client, baseURL, toolName string, arguments map[string]any, out io.Writer) error {
	payload, err := json.Marshal(callRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: callRequestParams{
			Name:      toolName,
			Arguments: arguments,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal call request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build call request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call %q failed: %w", toolName, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read call response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("call %q failed with HTTP %d: %s", toolName, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rpc callResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return fmt.Errorf("decode call response: %w", err)
	}
	if rpc.Error != nil {
		return fmt.Errorf("MCP error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}

	var result callResult
	if err := json.Unmarshal(rpc.Result, &result); err == nil && len(result.Content) > 0 {
		lines := make([]string, 0, len(result.Content))
		for _, item := range result.Content {
			if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
				lines = append(lines, item.Text)
			}
		}
		if len(lines) > 0 {
			fmt.Fprintln(out, strings.Join(lines, "\n"))
			return nil
		}
	}

	formatted, err := json.MarshalIndent(rpc.Result, "", "  ")
	if err != nil {
		return fmt.Errorf("format call result: %w", err)
	}
	fmt.Fprintln(out, string(formatted))
	return nil
}

func parseCallInput(args []string) (serverName string, toolName string, toolArgs []string, err error) {
	if len(args) == 0 {
		return "", "", nil, errors.New("usage: mcporter call <server> <tool> [arg=value...]")
	}

	if strings.Contains(args[0], ".") {
		parts := strings.SplitN(args[0], ".", 2)
		if parts[0] == "" || parts[1] == "" {
			return "", "", nil, errors.New("usage: mcporter call <server> <tool> [arg=value...]")
		}
		return parts[0], parts[1], args[1:], nil
	}

	if len(args) < 2 {
		return "", "", nil, errors.New("usage: mcporter call <server> <tool> [arg=value...]")
	}
	return args[0], args[1], args[2:], nil
}

func loadConfig(path string) (configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return configFile{}, err
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return configFile{}, fmt.Errorf("invalid config JSON: %w", err)
	}
	return cfg, nil
}

func sortedServerNames(cfg configFile) []string {
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func extractConfigPath(args []string) ([]string, string, error) {
	configPath := defaultConfigPath()
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != "--config" {
			remaining = append(remaining, args[i])
			continue
		}
		if i+1 >= len(args) {
			return nil, "", errors.New("--config requires a path")
		}
		configPath = args[i+1]
		i++
	}
	return remaining, configPath, nil
}

func commandSummary(cfg serverConfig) string {
	if len(cfg.Command.parts) == 0 {
		return ""
	}
	return strings.Join(cfg.Command.parts, " ")
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "mcporter-go")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mcporter list [config-path]")
	fmt.Fprintln(w, "  mcporter call <server> <tool> [arg=value...]")
	fmt.Fprintln(w, "  mcporter call <server.tool> [arg=value...]")
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  --config <path>")
	fmt.Fprintln(w, "  mcporter version")
}
