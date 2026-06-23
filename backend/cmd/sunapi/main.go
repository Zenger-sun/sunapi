package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/buildinfo"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/sunapi"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/sunstatic"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
	sdkapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	sdkauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	log "github.com/sirupsen/logrus"
	"github.com/skratchdot/open-golang/open"
)

func init() {
	logging.SetupBaseLogger()
	gin.SetMode(gin.ReleaseMode)
}

func main() {
	if isVersionCommand(os.Args[1:]) {
		printVersion()
		return
	}

	var dataDirFlag string
	var hostFlag string
	var portFlag int
	var noBrowser bool

	flag.StringVar(&dataDirFlag, "data", "", "Local data directory")
	flag.StringVar(&hostFlag, "host", "", "Override listen host")
	flag.IntVar(&portFlag, "port", 0, "Override listen port")
	flag.BoolVar(&noBrowser, "no-browser", false, "Do not open the browser after startup")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataDir, err := resolveDataDir(dataDirFlag)
	if err != nil {
		log.Fatalf("failed to resolve data directory: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	store, err := sunapi.OpenStore(filepath.Join(dataDir, "sunapi.db"))
	if err != nil {
		log.Fatalf("failed to open local database: %v", err)
	}
	defer store.Close()

	settings, err := applyFlagOverrides(ctx, store, hostFlag, portFlag)
	if err != nil {
		log.Fatalf("failed to load settings: %v", err)
	}

	configPath := filepath.Join(dataDir, "config.yaml")
	authDir := filepath.Join(dataDir, "auths")
	writer := sunapi.NewConfigWriter(store, configPath, authDir)
	cfg, err := writer.Write(ctx)
	if err != nil {
		log.Fatalf("failed to write proxy config: %v", err)
	}
	sunapi.RegisterSQLiteAPIKeyAccessProvider(store)

	distFS, err := sunstatic.Dist()
	if err != nil {
		log.Warnf("failed to load embedded frontend: %v", err)
		distFS = nil
	}

	sdkauth.RegisterTokenStore(sdkauth.NewFileTokenStore())
	service, err := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(configPath).
		WithoutPlugins().
		WithServerOptions(
			sdkapi.WithMiddleware(sunapi.BlockBundledManagementRoutes()),
			sdkapi.WithRouterConfigurator(func(engine *gin.Engine, base *handlers.BaseAPIHandler, _ *config.Config) {
				sunapi.RegisterRoutes(engine, store, writer)
				sunapi.RegisterPlaygroundRoutes(engine, store, base)
				if distFS != nil {
					sunapi.RegisterFrontend(engine, distFS)
				}
			}),
		).
		WithHooks(cliproxy.Hooks{
			OnAfterStart: func(*cliproxy.Service) {
				url := browserURL(cfg)
				homeURL := strings.TrimRight(url, "/") + "/home"
				consoleURL := strings.TrimRight(url, "/") + "/dashboard"
				fmt.Printf("SunAPI home: %s\n", homeURL)
				fmt.Printf("SunAPI console: %s\n", consoleURL)
				fmt.Printf("OpenAI-compatible endpoint: %s\n", url)
				if settings.AutoOpenBrowser && !noBrowser {
					go openBrowser(homeURL)
				}
			},
		}).
		Build()
	if err != nil {
		log.Fatalf("failed to build proxy service: %v", err)
	}
	service.RegisterUsagePlugin(sunapi.NewUsagePlugin(store))

	fmt.Println("SunAPI Local is starting")
	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Printf("Config file: %s\n", configPath)

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("proxy service exited: %v", err)
	}
}

func isVersionCommand(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "version", "--version", "-version":
		return true
	default:
		return false
	}
}

func printVersion() {
	fmt.Printf("SunAPI Version: %s\n", buildinfo.Version)
	fmt.Printf("Commit: %s\n", buildinfo.Commit)
	fmt.Printf("BuiltAt: %s\n", buildinfo.BuildDate)
}

func resolveDataDir(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if appDataDir, ok := macAppDataDir(); ok {
			raw = appDataDir
		} else if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
			raw = filepath.Join(filepath.Dir(exe), "data")
		} else {
			wd, wdErr := os.Getwd()
			if wdErr != nil {
				return "", wdErr
			}
			raw = filepath.Join(wd, "data")
		}
	}
	return filepath.Abs(raw)
}

func macAppDataDir() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	exePath := strings.ToLower(filepath.ToSlash(filepath.Clean(exe)))
	if !strings.Contains(exePath, ".app/contents/macos/") {
		return "", false
	}
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return "", false
	}
	return filepath.Join(configDir, "SunAPI"), true
}

func applyFlagOverrides(ctx context.Context, store *sunapi.Store, host string, port int) (sunapi.Settings, error) {
	settings, err := store.Settings(ctx)
	if err != nil {
		return sunapi.Settings{}, err
	}
	changed := false
	if host = strings.TrimSpace(host); host != "" {
		settings.ListenHost = host
		changed = true
	}
	if port > 0 {
		settings.ListenPort = port
		changed = true
	}
	if !changed {
		return settings, nil
	}
	return store.UpdateSettings(ctx, settings)
}

func browserURL(cfg *config.Config) string {
	host := "127.0.0.1"
	port := 8317
	if cfg != nil {
		if trimmed := strings.TrimSpace(cfg.Host); trimmed != "" {
			host = trimmed
		}
		if cfg.Port > 0 {
			port = cfg.Port
		}
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	if err := open.Run(url); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Warnf("failed to open browser: %v", err)
	}
}
