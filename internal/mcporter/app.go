package mcporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
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

	transport := "stdio"
	endpoint := commandSummary(srv)
	if strings.TrimSpace(srv.BaseURL) != "" {
		transport = "http"
		endpoint = srv.BaseURL
	}
	if endpoint == "" {
		endpoint = "(unspecified)"
	}

	if len(toolArgs) == 0 {
		fmt.Fprintf(out, "planned call %s.%s via %s %s\n", serverName, toolName, transport, endpoint)
		return nil
	}
	fmt.Fprintf(out, "planned call %s.%s via %s %s args=%s\n", serverName, toolName, transport, endpoint, strings.Join(toolArgs, " "))
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
