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
		path := defaultConfigPath()
		if len(args) > 1 {
			path = args[1]
		}
		if err := listServers(path, stdout); err != nil {
			fmt.Fprintf(stderr, "mcporter: %v\n", err)
			return 1
		}
		return 0
	case "call":
		if len(args) < 3 {
			fmt.Fprintln(stderr, "mcporter: usage: mcporter call <server> <tool>")
			return 1
		}
		fmt.Fprintf(stderr, "mcporter: call is not implemented yet in Go (requested %s.%s)\n", args[1], args[2])
		return 1
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
	Description string   `json:"description"`
	BaseURL     string   `json:"baseUrl"`
	Command     any      `json:"command"`
	Args        []string `json:"args"`
}

func listServers(path string, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return errors.New("invalid config JSON")
	}

	if len(cfg.MCPServers) == 0 {
		fmt.Fprintln(out, "No servers configured.")
		return nil
	}

	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
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

func commandSummary(cfg serverConfig) string {
	switch v := cfg.Command.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "mcporter-go")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  mcporter list [config-path]")
	fmt.Fprintln(w, "  mcporter call <server> <tool>")
	fmt.Fprintln(w, "  mcporter version")
}
