// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package main provides the entry point for the switchAILocal server.
// This server acts as a proxy that provides OpenAI/Gemini/Claude compatible API interfaces
// for CLI models, allowing CLI models to be used with tools and libraries designed for standard AI APIs.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	configaccess "github.com/traylinx/switchAILocal/internal/access/config_access"
	"github.com/traylinx/switchAILocal/internal/buildinfo"
	"github.com/traylinx/switchAILocal/internal/cmd"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/logging"
	"github.com/traylinx/switchAILocal/internal/managementasset"
	"github.com/traylinx/switchAILocal/internal/memory"
	"github.com/traylinx/switchAILocal/internal/misc"
	"github.com/traylinx/switchAILocal/internal/store"
	_ "github.com/traylinx/switchAILocal/internal/translator"
	"github.com/traylinx/switchAILocal/internal/usage"
	"github.com/traylinx/switchAILocal/internal/util"
	sdkAuth "github.com/traylinx/switchAILocal/sdk/auth"
	coreauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

var (
	Version           = "dev"
	Commit            = "none"
	BuildDate         = "unknown"
	DefaultConfigPath = ""
)

// init initializes the shared logger setup.
func init() {
	logging.SetupBaseLogger()
	buildinfo.Version = Version
	buildinfo.Commit = Commit
	buildinfo.BuildDate = BuildDate
}

// validateFilePath checks if a file path is safe and doesn't contain path traversal
func validateFilePath(path string) error {
	// Check for path traversal attempts
	if strings.Contains(path, "..") || strings.Contains(path, "~") {
		return fmt.Errorf("invalid file path: contains path traversal")
	}

	// Check for absolute paths that might be unsafe
	if filepath.IsAbs(path) {
		// Allow absolute paths if they are in the user's home directory
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
			return nil
		}

		// Otherwise ensure they're within expected system directories
		if !strings.HasPrefix(path, "/etc/") &&
			!strings.HasPrefix(path, "/usr/") &&
			!strings.HasPrefix(path, "/var/") {
			return fmt.Errorf("absolute paths must be in standard system directories (or user home)")
		}
	}

	// Check for null bytes or control characters
	for _, r := range path {
		if r < 32 && r != '/' && r != '.' && r != '_' && r != '-' {
			return fmt.Errorf("invalid file path: contains control characters")
		}
	}

	return nil
}

// sanitizeError removes sensitive information from error messages
func sanitizeError(err error, context string) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Remove common sensitive patterns
	sensitivePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(://[^:@]+):([^@]+)@`), // username:password@
		regexp.MustCompile(`\b(password|secret|token|key)=[^\s]+`),
		regexp.MustCompile(`\b(access|secret)[-_]?key[^\s]*\s*=\s*[^\s]+`),
	}

	for _, pattern := range sensitivePatterns {
		errStr = pattern.ReplaceAllString(errStr, "$1:***@")
	}

	return fmt.Errorf("%s: %s", context, errStr)
}

// checkFilePermissions ensures sensitive files have appropriate permissions
func checkFilePermissions(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	// Check if file permissions are too permissive
	mode := fileInfo.Mode()
	if mode.Perm()&0077 != 0 {
		return fmt.Errorf("file %s has insecure permissions (should be 600 or more restrictive)", filePath)
	}

	return nil
}

// validateEnvironmentVariables checks sensitive environment variables for security
func validateEnvironmentVariables() error {
	sensitiveVars := []string{
		"PGSTORE_DSN", "pgstore_dsn",
		"GITSTORE_GIT_TOKEN", "gitstore_git_token",
		"OBJECTSTORE_ACCESS_KEY", "objectstore_access_key",
		"OBJECTSTORE_SECRET_KEY", "objectstore_secret_key",
	}

	for _, varName := range sensitiveVars {
		if value, exists := os.LookupEnv(varName); exists {
			// Check for common insecure patterns
			if strings.Contains(value, "://") && strings.Contains(value, "@") {
				// Looks like a connection string with credentials
				parts := strings.Split(value, "@")
				if len(parts) > 1 && strings.Contains(parts[0], ":") {
					// Contains username:password - this is a warning, not an error
					log.Warnf("Environment variable %s contains credentials - consider using more secure credential management", varName)
				}
			}
		}
	}

	return nil
}

// main is the entry point of the application.
// It parses command-line flags, loads configuration, and starts the appropriate
// service based on the provided flags (login, codex-login, or server mode).
func main() {
	fmt.Printf("switchAILocal Version: %s, Commit: %s, BuiltAt: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)

	// Command-line flags to control the application's behavior.
	var login bool
	var codexLogin bool
	var claudeLogin bool
	var qwenLogin bool
	var vibeLogin bool
	var ollamaLogin bool
	var iflowLogin bool
	var iflowCookie bool
	var noBrowser bool
	var antigravityLogin bool
	var projectID string
	var vertexImport string
	var configPath string
	var password string

	// Define command-line flags for different operation modes.
	flag.BoolVar(&login, "login", false, "Login Google Account")
	flag.BoolVar(&codexLogin, "codex-login", false, "Login to Codex using OAuth")
	flag.BoolVar(&claudeLogin, "claude-login", false, "Login to Claude using OAuth")
	flag.BoolVar(&qwenLogin, "qwen-login", false, "Login to Qwen using OAuth")
	flag.BoolVar(&vibeLogin, "vibe-login", false, "Login to Vibe (Local)")
	flag.BoolVar(&ollamaLogin, "ollama-login", false, "Connect to local Ollama instance")
	flag.BoolVar(&iflowLogin, "iflow-login", false, "Login to iFlow using OAuth")
	flag.BoolVar(&iflowCookie, "iflow-cookie", false, "Login to iFlow using Cookie")
	flag.BoolVar(&noBrowser, "no-browser", false, "Don't open browser automatically for OAuth")
	flag.BoolVar(&antigravityLogin, "antigravity-login", false, "Login to Antigravity using OAuth")
	flag.StringVar(&projectID, "project_id", "", "Project ID (Gemini only, not required)")
	flag.StringVar(&configPath, "config", DefaultConfigPath, "Configure File Path")
	flag.StringVar(&vertexImport, "vertex-import", "", "Import Vertex service account key JSON file")
	flag.StringVar(&password, "password", "", "Server password (use environment variable for security)")

	flag.CommandLine.Usage = func() {
		out := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(out, "Usage of %s\n", os.Args[0])
		flag.CommandLine.VisitAll(func(f *flag.Flag) {
			if f.Name == "password" {
				return
			}
			s := fmt.Sprintf("  -%s", f.Name)
			name, unquoteUsage := flag.UnquoteUsage(f)
			if name != "" {
				s += " " + name
			}
			if len(s) <= 4 {
				s += "	"
			} else {
				s += "\n    "
			}
			if unquoteUsage != "" {
				s += unquoteUsage
			}
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				s += fmt.Sprintf(" (default %s)", f.DefValue)
			}
			_, _ = fmt.Fprint(out, s+"\n")
		})
	}

	// Parse the command-line flags.
	flag.Parse()

	// Check for memory subcommands before processing other flags
	if len(os.Args) > 1 && os.Args[1] == "memory" {
		handleMemoryCommand(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "heartbeat" {
		handleHeartbeatCommand(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "steering" {
		handleSteeringCommand(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "hooks" {
		handleHooksCommand(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "learning" {
		handleLearningCommand(os.Args[2:])
		return
	}

	// Validate environment variables for security issues
	if err := validateEnvironmentVariables(); err != nil {
		log.Errorf("environment validation failed: %v", err)
		return
	}

	// Core application variables.
	var err error
	var cfg *config.Config
	var isCloudDeploy bool
	var (
		usePostgresStore     bool
		pgStoreDSN           string
		pgStoreSchema        string
		pgStoreLocalPath     string
		pgStoreInst          *store.PostgresStore
		useGitStore          bool
		gitStoreRemoteURL    string
		gitStoreUser         string
		gitStorePassword     string
		gitStoreLocalPath    string
		gitStoreInst         *store.GitTokenStore
		gitStoreRoot         string
		useObjectStore       bool
		objectStoreEndpoint  string
		objectStoreAccess    string
		objectStoreSecret    string
		objectStoreBucket    string
		objectStoreLocalPath string
		objectStoreInst      *store.ObjectTokenStore
	)

	wd, err := os.Getwd()
	if err != nil {
		log.Errorf("failed to get working directory: %v", err)
		return
	}

	// Load environment variables from .env if present.
	if errLoad := godotenv.Load(filepath.Join(wd, ".env")); errLoad != nil {
		if !errors.Is(errLoad, os.ErrNotExist) {
			log.WithError(errLoad).Warn("failed to load .env file")
		}
	}

	lookupEnv := func(keys ...string) (string, bool) {
		for _, key := range keys {
			if value, ok := os.LookupEnv(key); ok {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed, true
				}
			}
		}
		return "", false
	}
	writableBase := util.WritablePath()
	if value, ok := lookupEnv("PGSTORE_DSN", "pgstore_dsn"); ok {
		usePostgresStore = true
		pgStoreDSN = value
	}
	if usePostgresStore {
		if value, ok := lookupEnv("PGSTORE_SCHEMA", "pgstore_schema"); ok {
			pgStoreSchema = value
		}
		if value, ok := lookupEnv("PGSTORE_LOCAL_PATH", "pgstore_local_path"); ok {
			pgStoreLocalPath = value
		}
		if pgStoreLocalPath == "" {
			if writableBase != "" {
				pgStoreLocalPath = writableBase
			} else {
				pgStoreLocalPath = wd
			}
		}
		useGitStore = false
	}
	if value, ok := lookupEnv("GITSTORE_GIT_URL", "gitstore_git_url"); ok {
		useGitStore = true
		gitStoreRemoteURL = value
	}
	if value, ok := lookupEnv("GITSTORE_GIT_USERNAME", "gitstore_git_username"); ok {
		gitStoreUser = value
	}
	if value, ok := lookupEnv("GITSTORE_GIT_TOKEN", "gitstore_git_token"); ok {
		gitStorePassword = value
	}
	if value, ok := lookupEnv("GITSTORE_LOCAL_PATH", "gitstore_local_path"); ok {
		gitStoreLocalPath = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_ENDPOINT", "objectstore_endpoint"); ok {
		useObjectStore = true
		objectStoreEndpoint = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_ACCESS_KEY", "objectstore_access_key"); ok {
		objectStoreAccess = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_SECRET_KEY", "objectstore_secret_key"); ok {
		objectStoreSecret = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_BUCKET", "objectstore_bucket"); ok {
		objectStoreBucket = value
	}
	if value, ok := lookupEnv("OBJECTSTORE_LOCAL_PATH", "objectstore_local_path"); ok {
		objectStoreLocalPath = value
	}

	// Check for cloud deploy mode only on first execution
	// Read env var name in uppercase: DEPLOY
	deployEnv := os.Getenv("DEPLOY")
	if deployEnv == "cloud" {
		isCloudDeploy = true
	}

	// Determine and load the configuration file.
	// Prefer the Postgres store when configured, otherwise fallback to git or local files.
	var configFilePath string
	if usePostgresStore {
		if pgStoreLocalPath == "" {
			pgStoreLocalPath = wd
		}
		pgStoreLocalPath = filepath.Join(pgStoreLocalPath, "pgstore")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		pgStoreInst, err = store.NewPostgresStore(ctx, store.PostgresStoreConfig{
			DSN:      pgStoreDSN,
			Schema:   pgStoreSchema,
			SpoolDir: pgStoreLocalPath,
		})
		cancel()
		if err != nil {
			log.Errorf("failed to initialize postgres token store: %v", sanitizeError(err, "postgres initialization"))
			return
		}
		examplePath := filepath.Join(wd, "config.example.yaml")
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		if errBootstrap := pgStoreInst.Bootstrap(ctx, examplePath); errBootstrap != nil {
			cancel()
			log.Errorf("failed to bootstrap postgres-backed config: %v", errBootstrap)
			return
		}
		cancel()
		configFilePath = pgStoreInst.ConfigPath()
		cfg, err = config.LoadConfigOptional(configFilePath, isCloudDeploy)
		if err == nil {
			cfg.AuthDir = pgStoreInst.AuthDir()
			log.Infof("postgres-backed token store enabled, workspace path: %s", pgStoreInst.WorkDir())
		}
	} else if useObjectStore {
		if objectStoreLocalPath == "" {
			if writableBase != "" {
				objectStoreLocalPath = writableBase
			} else {
				objectStoreLocalPath = wd
			}
		}
		objectStoreRoot := filepath.Join(objectStoreLocalPath, "objectstore")
		resolvedEndpoint := strings.TrimSpace(objectStoreEndpoint)
		useSSL := true
		if strings.Contains(resolvedEndpoint, "://") {
			parsed, errParse := url.Parse(resolvedEndpoint)
			if errParse != nil {
				log.Errorf("failed to parse object store endpoint %q: %v", objectStoreEndpoint, errParse)
				return
			}
			switch strings.ToLower(parsed.Scheme) {
			case "http":
				useSSL = false
			case "https":
				useSSL = true
			default:
				log.Errorf("unsupported object store scheme %q (only http and https are allowed)", parsed.Scheme)
				return
			}
			if parsed.Host == "" {
				log.Errorf("object store endpoint %q is missing host information", objectStoreEndpoint)
				return
			}
			resolvedEndpoint = parsed.Host
			if parsed.Path != "" && parsed.Path != "/" {
				resolvedEndpoint = strings.TrimSuffix(parsed.Host+parsed.Path, "/")
			}
		}
		resolvedEndpoint = strings.TrimRight(resolvedEndpoint, "/")
		objCfg := store.ObjectStoreConfig{
			Endpoint:  resolvedEndpoint,
			Bucket:    objectStoreBucket,
			AccessKey: objectStoreAccess,
			SecretKey: objectStoreSecret,
			LocalRoot: objectStoreRoot,
			UseSSL:    useSSL,
			PathStyle: true,
		}
		objectStoreInst, err = store.NewObjectTokenStore(objCfg)
		if err != nil {
			log.Errorf("failed to initialize object token store: %v", sanitizeError(err, "object store initialization"))
			return
		}
		examplePath := filepath.Join(wd, "config.example.yaml")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if errBootstrap := objectStoreInst.Bootstrap(ctx, examplePath); errBootstrap != nil {
			cancel()
			log.Errorf("failed to bootstrap object-backed config: %v", errBootstrap)
			return
		}
		cancel()
		configFilePath = objectStoreInst.ConfigPath()
		cfg, err = config.LoadConfigOptional(configFilePath, isCloudDeploy)
		if err == nil {
			if cfg == nil {
				cfg = &config.Config{}
			}
			cfg.AuthDir = objectStoreInst.AuthDir()
			log.Infof("object-backed token store enabled, bucket: %s", objectStoreBucket)
		}
	} else if useGitStore {
		if gitStoreLocalPath == "" {
			if writableBase != "" {
				gitStoreLocalPath = writableBase
			} else {
				gitStoreLocalPath = wd
			}
		}
		gitStoreRoot = filepath.Join(gitStoreLocalPath, "gitstore")
		authDir := filepath.Join(gitStoreRoot, "auths")
		gitStoreInst = store.NewGitTokenStore(gitStoreRemoteURL, gitStoreUser, gitStorePassword)
		gitStoreInst.SetBaseDir(authDir)
		if errRepo := gitStoreInst.EnsureRepository(); errRepo != nil {
			log.Errorf("failed to prepare git token store: %v", errRepo)
			return
		}
		configFilePath = gitStoreInst.ConfigPath()
		if configFilePath == "" {
			configFilePath = filepath.Join(gitStoreRoot, "config", "config.yaml")
		}
		if _, statErr := os.Stat(configFilePath); errors.Is(statErr, fs.ErrNotExist) {
			examplePath := filepath.Join(wd, "config.example.yaml")
			if _, errExample := os.Stat(examplePath); errExample != nil {
				log.Errorf("failed to find template config file: %v", errExample)
				return
			}
			if errCopy := misc.CopyConfigTemplate(examplePath, configFilePath); errCopy != nil {
				log.Errorf("failed to bootstrap git-backed config: %v", errCopy)
				return
			}
			if errCommit := gitStoreInst.PersistConfig(context.Background()); errCommit != nil {
				log.Errorf("failed to commit initial git-backed config: %v", errCommit)
				return
			}
			log.Infof("git-backed config initialized from template: %s", configFilePath)
		} else if statErr != nil {
			log.Errorf("failed to inspect git-backed config: %v", statErr)
			return
		}
		cfg, err = config.LoadConfigOptional(configFilePath, isCloudDeploy)
		if err == nil {
			cfg.AuthDir = gitStoreInst.AuthDir()
			log.Infof("git-backed token store enabled, repository path: %s", gitStoreRoot)
		}
	} else if configPath != "" {
		// Validate the config path for security
		if err := validateFilePath(configPath); err != nil {
			log.Errorf("invalid config path: %v", err)
			return
		}
		configFilePath = configPath
		cfg, err = config.LoadConfigOptional(configPath, isCloudDeploy)
	} else {
		wd, err = os.Getwd()
		if err != nil {
			log.Errorf("failed to get working directory: %v", err)
			return
		}
		configFilePath = filepath.Join(wd, "config.yaml")
		cfg, err = config.LoadConfigOptional(configFilePath, isCloudDeploy)
	}

	// Check file permissions for sensitive configuration files
	if cfg != nil && configFilePath != "" {
		if err := checkFilePermissions(configFilePath); err != nil {
			log.Warnf("security warning: %v", err)
			// Don't fail, but warn about insecure permissions
		}
	}

	if err != nil {
		log.Errorf("failed to load config: %v", sanitizeError(err, "config loading"))
		return
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	// In cloud deploy mode, check if we have a valid configuration
	var configFileExists bool
	if isCloudDeploy {
		if info, errStat := os.Stat(configFilePath); errStat != nil {
			// Don't mislead: API server will not start until configuration is provided.
			log.Info("Cloud deploy mode: No configuration file detected; standing by for configuration")
			configFileExists = false
		} else if info.IsDir() {
			log.Info("Cloud deploy mode: Config path is a directory; standing by for configuration")
			configFileExists = false
		} else if cfg.Port == 0 {
			// LoadConfigOptional returns empty config when file is empty or invalid.
			// Config file exists but is empty or invalid; treat as missing config
			log.Info("Cloud deploy mode: Configuration file is empty or invalid; standing by for valid configuration")
			configFileExists = false
		} else {
			log.Info("Cloud deploy mode: Configuration file detected; starting service")
			configFileExists = true
		}
	}
	usage.SetStatisticsEnabled(cfg.UsageStatisticsEnabled)
	coreauth.SetQuotaCooldownDisabled(cfg.DisableCooling)

	if err = logging.ConfigureLogOutput(cfg.LoggingToFile, cfg.LogsMaxTotalSizeMB); err != nil {
		log.Errorf("failed to configure log output: %v", err)
		return
	}

	log.Infof("switchAILocal Version: %s, Commit: %s, BuiltAt: %s", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate)

	// Set the log level based on the configuration.
	util.SetLogLevel(cfg)

	if resolvedAuthDir, errResolveAuthDir := util.ResolveAuthDir(cfg.AuthDir); errResolveAuthDir != nil {
		log.Errorf("failed to resolve auth directory: %v", errResolveAuthDir)
		return
	} else {
		// Validate auth directory path for security
		if err := validateFilePath(resolvedAuthDir); err != nil {
			log.Errorf("invalid auth directory path: %v", err)
			return
		}
		cfg.AuthDir = resolvedAuthDir

		// Check permissions on auth directory
		if err := checkFilePermissions(resolvedAuthDir); err != nil {
			log.Warnf("security warning for auth directory: %v", err)
		}
	}
	managementasset.SetCurrentConfig(cfg)

	// Create login options to be used in authentication flows.
	options := &cmd.LoginOptions{
		NoBrowser: noBrowser,
	}

	// Register the shared token store once so all components use the same persistence backend.
	if usePostgresStore {
		sdkAuth.RegisterTokenStore(pgStoreInst)
	} else if useObjectStore {
		sdkAuth.RegisterTokenStore(objectStoreInst)
	} else if useGitStore {
		sdkAuth.RegisterTokenStore(gitStoreInst)
	} else {
		sdkAuth.RegisterTokenStore(sdkAuth.NewFileTokenStore())
	}

	// Register built-in access providers before constructing services.
	configaccess.Register()

	// Handle different command modes based on the provided flags.

	if vertexImport != "" {
		// Handle Vertex service account import
		cmd.DoVertexImport(cfg, vertexImport)
	} else if login {
		// Handle Google/Gemini login
		cmd.DoLogin(cfg, projectID, options)
	} else if antigravityLogin {
		// Handle Antigravity login
		cmd.DoAntigravityLogin(cfg, options)
	} else if codexLogin {
		// Handle Codex login
		cmd.DoCodexLogin(cfg, options)
	} else if claudeLogin {
		// Handle Claude login
		cmd.DoClaudeLogin(cfg, options)
	} else if qwenLogin {
		cmd.DoQwenLogin(cfg, options)
	} else if vibeLogin {
		cmd.DoVibeLogin(cfg, options)
	} else if ollamaLogin {
		cmd.DoOllamaLogin(cfg, options)
	} else if iflowLogin {
		cmd.DoIFlowLogin(cfg, options)
	} else if iflowCookie {
		cmd.DoIFlowCookieAuth(cfg, options)
	} else {
		// In cloud deploy mode without config file, just wait for shutdown signals
		if isCloudDeploy && !configFileExists {
			// No config file available, just wait for shutdown
			cmd.WaitForCloudDeploy()
			return
		}

		// Secure password handling - clear from memory after use
		if password != "" {
			log.Info("Using password authentication (ensure this is a secure environment)")
			// Note: In production, consider using a more secure password handling mechanism
			// such as reading from a secure vault or environment variable
		}

		// Start the main proxy service
		managementasset.StartAutoUpdater(context.Background(), configFilePath)
		cmd.StartService(cfg, configFilePath, password)

		// Clear password from memory after service starts
		if password != "" {
			// Overwrite password in memory
			for i := range password {
				password = password[:i] + "*" + password[i+1:]
			}
			password = ""
		}
	}
}

// handleMemoryCommand processes memory subcommands
func handleMemoryCommand(args []string) {
	// Parse memory command options
	opts, err := ParseMemoryCommand(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		printMemoryUsage()
		os.Exit(1)
	}

	// Load minimal configuration for memory commands
	// We need this to determine the memory base directory
	var cfg *config.Config

	// Try to load existing config
	wd, err := os.Getwd()
	if err != nil {
		log.Errorf("failed to get working directory: %v", err)
		os.Exit(1)
	}

	configFilePath := filepath.Join(wd, "config.yaml")
	cfg, err = config.LoadConfigOptional(configFilePath, false)
	if err != nil {
		// If config loading fails, use empty config
		cfg = &config.Config{}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Execute memory command
	DoMemoryCommand(cfg, opts)
}

// MemoryCommand represents the available memory subcommands
type MemoryCommand string

const (
	MemoryInit        MemoryCommand = "init"
	MemoryStatus      MemoryCommand = "status"
	MemoryHistory     MemoryCommand = "history"
	MemoryPreferences MemoryCommand = "preferences"
	MemoryReset       MemoryCommand = "reset"
	MemoryExport      MemoryCommand = "export"
)

// MemoryOptions holds the command-line options for memory commands
type MemoryOptions struct {
	Command    MemoryCommand
	Limit      int
	APIKey     string
	APIKeyHash string
	Confirm    bool
	Output     string
	Format     string
}

// DoMemoryCommand executes the specified memory command with the given options
func DoMemoryCommand(cfg *config.Config, opts *MemoryOptions) {
	switch opts.Command {
	case MemoryInit:
		doMemoryInit(cfg)
	case MemoryStatus:
		doMemoryStatus(cfg)
	case MemoryHistory:
		doMemoryHistory(cfg, opts)
	case MemoryPreferences:
		doMemoryPreferences(cfg, opts)
	case MemoryReset:
		doMemoryReset(cfg, opts)
	case MemoryExport:
		doMemoryExport(cfg, opts)
	default:
		fmt.Printf("Unknown memory command: %s\n", opts.Command)
		printMemoryUsage()
		os.Exit(1)
	}
}

// doMemoryInit initializes the memory system
func doMemoryInit(cfg *config.Config) {
	fmt.Println("Initializing switchAILocal memory system...")

	// Create memory configuration
	memoryConfig := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       getMemoryBaseDir(cfg),
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	// Initialize memory manager
	manager, err := memory.NewMemoryManager(memoryConfig)
	if err != nil {
		log.Errorf("Failed to initialize memory system: %v", err)
		os.Exit(1)
	}
	defer manager.Close()

	fmt.Printf("✓ Memory system initialized successfully\n")
	fmt.Printf("  Base directory: %s\n", memoryConfig.BaseDir)
	fmt.Printf("  Retention: %d days\n", memoryConfig.RetentionDays)
	fmt.Printf("  Compression: %v\n", memoryConfig.Compression)
	fmt.Printf("  Max log size: %d MB\n", memoryConfig.MaxLogSizeMB)

	// Display directory structure
	fmt.Println("\nDirectory structure created:")
	fmt.Printf("  %s/\n", memoryConfig.BaseDir)
	fmt.Printf("  ├── routing-history.jsonl\n")
	fmt.Printf("  ├── provider-quirks.md\n")
	fmt.Printf("  ├── user-preferences/\n")
	fmt.Printf("  ├── daily/\n")
	fmt.Printf("  └── analytics/\n")

	fmt.Println("\nMemory system is ready to use!")
}

// doMemoryStatus shows memory system health and disk usage
func doMemoryStatus(cfg *config.Config) {
	fmt.Println("switchAILocal Memory System Status")
	fmt.Println("==================================")

	// Create memory configuration
	memoryConfig := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       getMemoryBaseDir(cfg),
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	// Check if memory system exists
	if _, err := os.Stat(memoryConfig.BaseDir); os.IsNotExist(err) {
		fmt.Println("❌ Memory system not initialized")
		fmt.Println("   Run 'switchAILocal memory init' to initialize")
		return
	}

	// Initialize memory manager
	manager, err := memory.NewMemoryManager(memoryConfig)
	if err != nil {
		log.Errorf("Failed to initialize memory manager: %v", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Get memory statistics
	stats, err := manager.GetStats()
	if err != nil {
		log.Errorf("Failed to get memory statistics: %v", err)
		os.Exit(1)
	}

	// Display status
	fmt.Printf("Status: ✓ Healthy\n")
	fmt.Printf("Base Directory: %s\n", memoryConfig.BaseDir)
	fmt.Printf("Enabled: %v\n", memoryConfig.Enabled)
	fmt.Println()

	// Display statistics
	fmt.Println("Statistics:")
	fmt.Printf("  Total Routing Decisions: %d\n", stats.TotalDecisions)
	fmt.Printf("  Total Users: %d\n", stats.TotalUsers)
	fmt.Printf("  Total Provider Quirks: %d\n", stats.TotalQuirks)
	fmt.Printf("  Disk Usage: %s\n", formatBytes(stats.DiskUsageBytes))

	if !stats.NewestDecision.IsZero() {
		fmt.Printf("  Newest Decision: %s\n", stats.NewestDecision.Format(time.RFC3339))
	}
	if !stats.OldestDecision.IsZero() {
		fmt.Printf("  Oldest Decision: %s\n", stats.OldestDecision.Format(time.RFC3339))
	}

	fmt.Println()

	// Display configuration
	fmt.Println("Configuration:")
	fmt.Printf("  Retention Days: %d\n", stats.RetentionDays)
	fmt.Printf("  Compression Enabled: %v\n", stats.CompressionEnabled)

	if !stats.LastCleanup.IsZero() {
		fmt.Printf("  Last Cleanup: %s\n", stats.LastCleanup.Format(time.RFC3339))
	}

	// Display daily logs statistics if available
	if stats.DailyLogsStats != nil {
		fmt.Println()
		fmt.Println("Daily Logs:")
		fmt.Printf("  Total Files: %d\n", stats.DailyLogsStats.TotalLogFiles)
		fmt.Printf("  Total Entries: %d\n", stats.DailyLogsStats.TotalEntries)
		fmt.Printf("  Disk Usage: %s\n", formatBytes(stats.DailyLogsStats.DiskUsageBytes))
	}

	// Display analytics information
	if !stats.LastAnalyticsUpdate.IsZero() {
		fmt.Println()
		fmt.Printf("Last Analytics Update: %s\n", stats.LastAnalyticsUpdate.Format(time.RFC3339))
	}
}

// doMemoryHistory displays recent routing decisions
func doMemoryHistory(cfg *config.Config, opts *MemoryOptions) {
	// Create memory configuration
	memoryConfig := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       getMemoryBaseDir(cfg),
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	// Initialize memory manager
	manager, err := memory.NewMemoryManager(memoryConfig)
	if err != nil {
		log.Errorf("Failed to initialize memory manager: %v", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Set default limit if not specified
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	// Get routing history
	var decisions []*memory.RoutingDecision
	if opts.APIKeyHash != "" {
		decisions, err = manager.GetHistory(opts.APIKeyHash, limit)
	} else {
		decisions, err = manager.GetAllHistory(limit)
	}

	if err != nil {
		log.Errorf("Failed to get routing history: %v", err)
		os.Exit(1)
	}

	// Display header
	fmt.Printf("Recent Routing Decisions (limit: %d)\n", limit)
	fmt.Println("=====================================")

	if len(decisions) == 0 {
		fmt.Println("No routing decisions found.")
		return
	}

	// Display decisions
	for i, decision := range decisions {
		fmt.Printf("\n[%d] %s\n", i+1, decision.Timestamp.Format(time.RFC3339))
		fmt.Printf("    API Key: %s\n", maskAPIKeyHash(decision.APIKeyHash))
		fmt.Printf("    Model: %s → %s\n", decision.Request.Model, decision.Routing.SelectedModel)
		fmt.Printf("    Intent: %s\n", decision.Request.Intent)
		fmt.Printf("    Tier: %s (confidence: %.2f)\n", decision.Routing.Tier, decision.Routing.Confidence)
		fmt.Printf("    Latency: %dms\n", decision.Routing.LatencyMs)

		status := "✓"
		if !decision.Outcome.Success {
			status = "✗"
		}
		fmt.Printf("    Outcome: %s Success: %v", status, decision.Outcome.Success)
		if decision.Outcome.ResponseTimeMs > 0 {
			fmt.Printf(", Response: %dms", decision.Outcome.ResponseTimeMs)
		}
		if decision.Outcome.QualityScore > 0 {
			fmt.Printf(", Quality: %.2f", decision.Outcome.QualityScore)
		}
		if decision.Outcome.Error != "" {
			fmt.Printf(", Error: %s", decision.Outcome.Error)
		}
		fmt.Println()
	}

	fmt.Printf("\nShowing %d of %d decisions\n", len(decisions), len(decisions))
}

// doMemoryPreferences displays user preferences for an API key
func doMemoryPreferences(cfg *config.Config, opts *MemoryOptions) {
	// Validate API key or hash is provided
	if opts.APIKey == "" && opts.APIKeyHash == "" {
		fmt.Println("Error: --api-key or --api-key-hash must be provided")
		os.Exit(1)
	}

	// Create memory configuration
	memoryConfig := &memory.MemoryConfig{
		Enabled:       true,
		BaseDir:       getMemoryBaseDir(cfg),
		RetentionDays: 90,
		MaxLogSizeMB:  100,
		Compression:   true,
	}

	// Initialize memory manager
	manager, err := memory.NewMemoryManager(memoryConfig)
	if err != nil {
		log.Errorf("Failed to initialize memory manager: %v", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Get API key hash
	apiKeyHash := opts.APIKeyHash
	if opts.APIKey != "" {
		apiKeyHash = hashAPIKey(opts.APIKey)
	}

	// Get user preferences
	preferences, err := manager.GetUserPreferences(apiKeyHash)
	if err != nil {
		log.Errorf("Failed to get user preferences: %v", err)
		os.Exit(1)
	}

	// Display preferences
	fmt.Printf("User Preferences for API Key: %s\n", maskAPIKeyHash(apiKeyHash))
	fmt.Println("==========================================")
	fmt.Printf("Last Updated: %s\n", preferences.LastUpdated.Format(time.RFC3339))
	fmt.Println()

	// Model preferences
	if len(preferences.ModelPreferences) > 0 {
		fmt.Println("Model Preferences:")
		for intent, model := range preferences.ModelPreferences {
			fmt.Printf("  %s → %s\n", intent, model)
		}
		fmt.Println()
	}

	// Provider bias
	if len(preferences.ProviderBias) > 0 {
		fmt.Println("Provider Bias:")
		for provider, bias := range preferences.ProviderBias {
			biasStr := fmt.Sprintf("%.2f", bias)
			if bias > 0 {
				biasStr = "+" + biasStr
			}
			fmt.Printf("  %s: %s\n", provider, biasStr)
		}
		fmt.Println()
	}

	// Custom rules
	if len(preferences.CustomRules) > 0 {
		fmt.Println("Custom Rules:")
		for i, rule := range preferences.CustomRules {
			fmt.Printf("  [%d] %s → %s (priority: %d)\n", i+1, rule.Condition, rule.Model, rule.Priority)
		}
		fmt.Println()
	}

	if len(preferences.ModelPreferences) == 0 && len(preferences.ProviderBias) == 0 && len(preferences.CustomRules) == 0 {
		fmt.Println("No preferences learned yet.")
		fmt.Println("Preferences will be learned automatically as you use switchAILocal.")
	}
}

// doMemoryReset clears all memory data with confirmation
func doMemoryReset(cfg *config.Config, opts *MemoryOptions) {
	if !opts.Confirm {
		fmt.Println("⚠️  WARNING: This will permanently delete all memory data including:")
		fmt.Println("  • Routing history")
		fmt.Println("  • User preferences")
		fmt.Println("  • Provider quirks")
		fmt.Println("  • Daily logs")
		fmt.Println("  • Analytics data")
		fmt.Println()
		fmt.Println("Use --confirm flag to proceed with reset.")
		fmt.Println()
		fmt.Println("💡 Tip: Use 'switchAILocal memory export' to create a backup first.")
		return
	}

	// Get memory base directory
	baseDir := getMemoryBaseDir(cfg)

	// Check if memory directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		fmt.Println("Memory system not found - nothing to reset.")
		return
	}

	// Create automatic backup before reset
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("memory-backup-before-reset-%s.tar.gz", timestamp)
	
	fmt.Println("Creating automatic backup before reset...")
	fmt.Printf("Backup file: %s\n", backupPath)
	
	// Create backup
	if err := createMemoryBackup(cfg, backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Error: Failed to create backup: %v\n", err)
		fmt.Fprintf(os.Stderr, "Reset cancelled for safety. Your data is preserved.\n")
		fmt.Fprintf(os.Stderr, "\n💡 Tip: Ensure you have write permissions and sufficient disk space.\n")
		os.Exit(1)
	}
	
	fmt.Printf("✓ Backup created successfully: %s\n\n", backupPath)
	fmt.Println("Proceeding with reset...")

	// Remove the entire memory directory
	if err := os.RemoveAll(baseDir); err != nil {
		log.Errorf("Failed to remove memory directory: %v", err)
		fmt.Fprintf(os.Stderr, "\n❌ Reset failed, but your data is safe in the backup: %s\n", backupPath)
		os.Exit(1)
	}

	fmt.Printf("✓ Memory system reset successfully\n")
	fmt.Printf("  Removed directory: %s\n", baseDir)
	fmt.Printf("  Backup available: %s\n", backupPath)
	fmt.Println("\n💡 Run 'switchAILocal memory init' to reinitialize the memory system.")
	fmt.Printf("💡 To restore from backup: tar -xzf %s -C ~/\n", backupPath)
}

// doMemoryExport creates a backup of all memory data
func doMemoryExport(cfg *config.Config, opts *MemoryOptions) {
	// Set default output filename if not provided
	output := opts.Output
	if output == "" {
		timestamp := time.Now().Format("20060102-150405")
		output = fmt.Sprintf("switchailocal-memory-%s.tar.gz", timestamp)
	}

	// Get memory base directory
	baseDir := getMemoryBaseDir(cfg)

	// Check if memory directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		fmt.Println("Memory system not found - nothing to export.")
		return
	}

	fmt.Printf("Exporting memory data to: %s\n", output)

	// Create output file
	outFile, err := os.Create(output)
	if err != nil {
		log.Errorf("Failed to create output file: %v", err)
		os.Exit(1)
	}
	defer outFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through memory directory and add files to archive
	err = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		// Create tar header
		header := &tar.Header{
			Name:    relPath,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open and copy file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})

	if err != nil {
		log.Errorf("Failed to create archive: %v", err)
		os.Exit(1)
	}

	// Get file size
	if stat, err := os.Stat(output); err == nil {
		fmt.Printf("✓ Export completed successfully\n")
		fmt.Printf("  Archive size: %s\n", formatBytes(stat.Size()))
		fmt.Printf("  Contains all memory data from: %s\n", baseDir)
	}
}

// createMemoryBackup creates a backup of memory data (used internally)
func createMemoryBackup(cfg *config.Config, output string) error {
	// Get memory base directory
	baseDir := getMemoryBaseDir(cfg)

	// Check if memory directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return fmt.Errorf("memory system not found")
	}

	// Create output file
	outFile, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through memory directory and add files to archive
	err = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		// Create tar header
		header := &tar.Header{
			Name:    relPath,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open and copy file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	return nil
}

// Helper functions

// getMemoryBaseDir returns the base directory for memory storage
func getMemoryBaseDir(cfg *config.Config) string {
	if cfg != nil && cfg.AuthDir != "" {
		return filepath.Join(cfg.AuthDir, "memory")
	}

	// Default to user home directory + .switchailocal/memory
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory is not accessible
		wd, _ := os.Getwd()
		return filepath.Join(wd, ".switchailocal", "memory")
	}
	return filepath.Join(home, ".switchailocal", "memory")
}

// hashAPIKey creates a SHA-256 hash of an API key
func hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return fmt.Sprintf("sha256:%x", hash)
}

// maskAPIKeyHash masks an API key hash for display
func maskAPIKeyHash(hash string) string {
	if len(hash) < 16 {
		return hash
	}
	return hash[:16] + "..."
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printMemoryUsage prints usage information for memory commands
func printMemoryUsage() {
	fmt.Println("Usage: switchAILocal memory <command> [options]")
	fmt.Println()
	fmt.Println("Available commands:")
	fmt.Println("  init                     Initialize memory system")
	fmt.Println("  status                   Show memory system health and disk usage")
	fmt.Println("  history                  View recent routing decisions")
	fmt.Println("  preferences              View user preferences")
	fmt.Println("  reset                    Clear all memory data")
	fmt.Println("  export                   Backup memory data")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --limit <n>              Limit number of history entries (default: 100)")
	fmt.Println("  --api-key <key>          API key for preferences lookup")
	fmt.Println("  --api-key-hash <hash>    API key hash for preferences lookup")
	fmt.Println("  --confirm                Confirm destructive operations")
	fmt.Println("  --output <file>          Output file for export (default: auto-generated)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  switchAILocal memory init")
	fmt.Println("  switchAILocal memory status")
	fmt.Println("  switchAILocal memory history --limit 50")
	fmt.Println("  switchAILocal memory preferences --api-key sk-test-123")
	fmt.Println("  switchAILocal memory reset --confirm")
	fmt.Println("  switchAILocal memory export --output backup.tar.gz")
}

// ParseMemoryCommand parses memory command from command line arguments
func ParseMemoryCommand(args []string) (*MemoryOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no memory command specified")
	}

	opts := &MemoryOptions{
		Limit:  100,    // default limit
		Format: "text", // default format
	}

	// Parse command
	switch args[0] {
	case "init":
		opts.Command = MemoryInit
	case "status":
		opts.Command = MemoryStatus
	case "history":
		opts.Command = MemoryHistory
	case "preferences":
		opts.Command = MemoryPreferences
	case "reset":
		opts.Command = MemoryReset
	case "export":
		opts.Command = MemoryExport
	default:
		return nil, fmt.Errorf("unknown memory command: %s", args[0])
	}

	// Parse options
	for i := 1; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "--limit":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--limit requires a value")
			}
			limit, err := strconv.Atoi(args[i+1])
			if err != nil {
				return nil, fmt.Errorf("invalid limit value: %s", args[i+1])
			}
			opts.Limit = limit
			i++ // skip next argument

		case "--api-key":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--api-key requires a value")
			}
			opts.APIKey = args[i+1]
			i++ // skip next argument

		case "--api-key-hash":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--api-key-hash requires a value")
			}
			opts.APIKeyHash = args[i+1]
			i++ // skip next argument

		case "--confirm":
			opts.Confirm = true

		case "--output":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--output requires a value")
			}
			opts.Output = args[i+1]
			i++ // skip next argument

		case "--format":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--format requires a value")
			}
			opts.Format = args[i+1]
			i++ // skip next argument

		default:
			return nil, fmt.Errorf("unknown option: %s", arg)
		}
	}

	return opts, nil
}

// handleHeartbeatCommand processes heartbeat subcommands
