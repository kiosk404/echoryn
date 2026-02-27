package init

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	cmdutil "github.com/kiosk404/echoryn/internal/echoadm/cmd/util"
	admconfig "github.com/kiosk404/echoryn/internal/echoadm/config"
	"github.com/kiosk404/echoryn/internal/echoadm/globals"
	"github.com/kiosk404/echoryn/internal/echoadm/utils/templates"
	"github.com/kiosk404/echoryn/pkg/cli/genericclioptions"
	"github.com/kiosk404/echoryn/pkg/utils/homedir"
	"github.com/kiosk404/echoryn/pkg/utils/json"
	"github.com/spf13/cobra"
)

var hivemindExample = templates.Examples(`
		# Initialize a Hivemind control plane with defaults
		echoadm init hivemind

		# Initialize with custom ports and data directory
		echoadm init hivemind --bind-port 8080 --grpc-port 8081 --data-dir /var/echoryn

		# Initialize from an existing config file
		echoadm init hivemind --from-config /path/to/hivemind-server.json

		# Force re-initialization
		echoadm init hivemind --force
`)

// InitHivemind holds options for the 'init hivemind' command.
type InitHivemind struct {
	BindAddress string
	BindPort    int
	GRPCPort    int
	DataDir     string
	AuthToken   string
	Mode        string
	FromConfig  string
	Force       bool

	genericclioptions.IOStreams
}

func NewCmdInitHivemind(_ cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := &InitHivemind{
		BindAddress: "0.0.0.0",
		BindPort:    11789,
		GRPCPort:    11788,
		DataDir:     filepath.Join(homedir.HomeDir(), admconfig.DefaultConfigDir, "hivemind"),
		Mode:        "debug",
		IOStreams:   ioStreams,
	}

	cmd := &cobra.Command{
		Use:     "hivemind",
		Short:   "Initialize a Hivemind control plane node",
		Long:    `Initialize this machine as a Hivemind control plane node. This creates the required directory structure, generates configuration files, creates an authentication token, and produces a join token for Golem workers.`,
		Example: hivemindExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringVar(&o.BindAddress, "bind-address", o.BindAddress, "HTTP API bind address")
	cmd.Flags().IntVar(&o.BindPort, "bind-port", o.BindPort, "HTTP API bind port")
	cmd.Flags().IntVar(&o.GRPCPort, "grpc-port", o.GRPCPort, "gRPC bind port")
	cmd.Flags().StringVar(&o.DataDir, "data-dir", o.DataDir, "Data directory for Hivemind")
	cmd.Flags().StringVar(&o.AuthToken, "auth-token", o.AuthToken, "Gateway auth token (auto-generated if not set)")
	cmd.Flags().StringVar(&o.Mode, "mode", o.Mode, "Server mode (debug|release)")
	cmd.Flags().StringVar(&o.FromConfig, "from-config", o.FromConfig, "Initialize from an existing hivemind-server.json")
	cmd.Flags().BoolVar(&o.Force, "force", o.Force, "Force re-initialization even if already initialized")

	return cmd
}

func (o *InitHivemind) Validate() error {
	if o.BindPort < 1 || o.BindPort > 65535 {
		return fmt.Errorf("--bind-port must be between 1 and 65535, got %d", o.BindPort)
	}
	if o.GRPCPort < 1 || o.GRPCPort > 65535 {
		return fmt.Errorf("--grpc-port must be between 1 and 65535, got %d", o.GRPCPort)
	}
	if o.BindPort == o.GRPCPort {
		return fmt.Errorf("--bind-port and --grpc-port must be different")
	}
	if o.Mode != "debug" && o.Mode != "release" {
		return fmt.Errorf("--mode must be 'debug' or 'release', got %q", o.Mode)
	}
	return nil
}

func (o *InitHivemind) Run(ctx context.Context) error {
	configPath := globals.ConfigPath
	if configPath == "" {
		configPath = admconfig.DefaultConfigPath()
	}

	// Check if already initialized.
	if admconfig.Exists(configPath) && !o.Force {
		return fmt.Errorf("echoryn is already initialized at %s. Use --force to re-initialize", configPath)
	}

	fmt.Fprintf(o.Out, "\n[init] Initializing Hivemind control plane...\n\n")

	// 1. Run pre-flight checks.
	fmt.Fprintf(o.Out, "[preflight] Running pre-flight checks...\n")
	if err := o.preflightChecks(); err != nil {
		return fmt.Errorf("[preflight] FAILED: %w", err)
	}
	fmt.Fprintf(o.Out, "[preflight] All checks passed.\n\n")

	// 2. Create directory structure.
	dirs := []struct {
		label string
		path  string
	}{
		{"base directory", admconfig.DefaultBaseDir()},
		{"hivemind data", o.DataDir},
		{"hivemind data/agents", filepath.Join(o.DataDir, "agents")},
		{"hivemind data/memory", filepath.Join(o.DataDir, "memory")},
	}
	for _, d := range dirs {
		fmt.Fprintf(o.Out, "[dirs] Creating %s: %s\n", d.label, d.path)
		if err := os.MkdirAll(d.path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d.label, err)
		}
	}
	fmt.Fprintln(o.Out)

	// 3. Generate auth token if not provided.
	if o.AuthToken == "" {
		o.AuthToken = generateToken("eid")
		fmt.Fprintf(o.Out, "[auth] Generated gateway auth token: %s\n", o.AuthToken)
	} else {
		fmt.Fprintf(o.Out, "[auth] Using provided auth token.\n")
	}

	// 4. Generate join token for golem workers.
	joinToken := generateToken("eidj")
	fmt.Fprintf(o.Out, "[token] Generated join token: %s\n\n", joinToken)

	// 5. Generate hivemind-server.json.
	serverConfigPath := filepath.Join(o.DataDir, "server.json")
	if o.FromConfig != "" {
		// Copy from existing config.
		data, err := os.ReadFile(o.FromConfig)
		if err != nil {
			return fmt.Errorf("read --from-config: %w", err)
		}
		if err := os.WriteFile(serverConfigPath, data, 0o644); err != nil {
			return fmt.Errorf("write server config: %w", err)
		}
		fmt.Fprintf(o.Out, "[config] Copied server config from %s\n", o.FromConfig)
	} else {
		serverCfg := o.buildServerConfig()
		data, _ := json.MarshalIndent(serverCfg, "", "  ")
		if err := os.WriteFile(serverConfigPath, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write server config: %w", err)
		}
		fmt.Fprintf(o.Out, "[config] Generated server config: %s\n", serverConfigPath)
	}

	// 6. Generate MCP config.
	mcpConfigPath := filepath.Join(o.DataDir, "mcp.json")
	mcpCfg := map[string]interface{}{"mcpServers": map[string]interface{}{}}
	mcpData, _ := json.MarshalIndent(mcpCfg, "", "  ")
	if err := os.WriteFile(mcpConfigPath, append(mcpData, '\n'), 0o644); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}

	// 7. Save tokens.json.
	tokensPath := filepath.Join(o.DataDir, "tokens.json")
	tokens := []map[string]interface{}{
		{
			"id":          generateTokenID(),
			"token":       joinToken,
			"description": "Auto-generated join token",
			"created_at":  time.Now().UTC().Format(time.RFC3339),
			"expires_at":  time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	tokensData, _ := json.MarshalIndent(tokens, "", "  ")
	if err := os.WriteFile(tokensPath, append(tokensData, '\n'), 0o644); err != nil {
		return fmt.Errorf("write tokens: %w", err)
	}

	// 8. Write global config.json.
	httpAddr := net.JoinHostPort(o.BindAddress, strconv.Itoa(o.BindPort))
	grpcAddr := net.JoinHostPort(o.BindAddress, strconv.Itoa(o.GRPCPort))

	cfg := admconfig.DefaultConfig()
	cfg.Node.Role = "hivemind"
	cfg.Node.Name = getHostname()
	cfg.Node.ID = generateTokenID()
	cfg.Hivemind.Address = httpAddr
	cfg.Hivemind.GRPCAddress = grpcAddr
	cfg.Hivemind.AuthToken = o.AuthToken
	cfg.Hivemind.DataDir = o.DataDir
	cfg.Hivemind.ConfigPath = serverConfigPath

	if err := admconfig.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(o.Out, "[config] Saved global config: %s\n", configPath)

	// 9. Print summary.
	fmt.Fprintf(o.Out, "\n%s\n", "============================================")
	fmt.Fprintf(o.Out, " Hivemind initialized successfully!\n")
	fmt.Fprintf(o.Out, "%s\n\n", "============================================")
	fmt.Fprintf(o.Out, "  HTTP API:    %s\n", httpAddr)
	fmt.Fprintf(o.Out, "  gRPC:        %s\n", grpcAddr)
	fmt.Fprintf(o.Out, "  Auth Token:  %s\n", o.AuthToken)
	fmt.Fprintf(o.Out, "  Data Dir:    %s\n", o.DataDir)
	fmt.Fprintf(o.Out, "  Config:      %s\n", configPath)
	fmt.Fprintf(o.Out, "\nTo start the Hivemind server:\n")
	fmt.Fprintf(o.Out, "  hivemind --echoryn.config %s\n", serverConfigPath)
	fmt.Fprintf(o.Out, "\nTo join a Golem worker to this Hivemind:\n")
	fmt.Fprintf(o.Out, "  echoadm join --token %s --hivemind-addr %s\n\n", joinToken, grpcAddr)

	return nil
}

func (o *InitHivemind) preflightChecks() error {
	// Check port availability.
	for _, port := range []int{o.BindPort, o.GRPCPort} {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return fmt.Errorf("port %d is already in use: %w", port, err)
		}
		ln.Close()
	}
	return nil
}

func (o *InitHivemind) buildServerConfig() map[string]interface{} {
	return map[string]interface{}{
		"grpc": map[string]interface{}{
			"bind-address": o.BindAddress,
			"bind-port":    o.GRPCPort,
			"max-msg-size": 4194304,
		},
		"serving": map[string]interface{}{
			"mode":         o.Mode,
			"healthz":      true,
			"middlewares":  []interface{}{},
			"bind-address": o.BindAddress,
			"bind-port":    o.BindPort,
		},
		"models": map[string]interface{}{
			"mode":             "merge",
			"default_provider": "deepseek",
			"default_model":    "deepseek-chat",
			"providers":        map[string]interface{}{},
		},
		"plugins": map[string]interface{}{
			"enabled": true,
			"slots": map[string]interface{}{
				"memory": "memory-core",
			},
			"entries": map[string]interface{}{
				"memory-core": map[string]interface{}{
					"config": map[string]interface{}{
						"enabled":            true,
						"workspace_dir":      ".",
						"db_path":            ".echoryn/memory/index.db",
						"embedding_provider": "openai",
						"embedding_model":    "text-embedding-3-small",
					},
				},
				"diagnostics": map[string]interface{}{
					"config": map[string]interface{}{"enabled": false},
				},
				"llm-task": map[string]interface{}{
					"config": map[string]interface{}{"enabled": true},
				},
			},
		},
	}
}

func generateToken(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func generateTokenID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getHostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "node-01"
	}
	return h
}
