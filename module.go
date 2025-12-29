package testingwindowsipc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
)

var (
	LoggingIpc       = resource.NewModel("bill", "testing-windows-ipc", "logging-ipc")
	errUnimplemented = errors.New("unimplemented")
)

func init() {
	resource.RegisterService(generic.API, LoggingIpc,
		resource.Registration[resource.Resource, *Config]{
			Constructor: newTestingWindowsIpcLoggingIpc,
		},
	)
}

type Config struct {
	// Desktop shortcut behavior.
	ShortcutName string `json:"shortcut_name,omitempty"` // default: "Viam Logging IPC"
	Message      string `json:"message,omitempty"`       // default: "hello from desktop shortcut"

	// HTTP endpoint the helpers post to.
	// Example: http://127.0.0.1:17831/log
	Endpoint string `json:"endpoint,omitempty"`
}

// Validate ensures all parts of the config are valid and important fields exist.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	return nil, nil, nil
}

type testingWindowsIpcLoggingIpc struct {
	resource.AlwaysRebuild

	name   resource.Name
	logger logging.Logger
	cfg    *Config

	cancelCtx  context.Context
	cancelFunc func()

	// Best-effort (only visible if running in an interactive user session).
	trayCmd *exec.Cmd
}

func newTestingWindowsIpcLoggingIpc(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (resource.Resource, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}
	return NewLoggingIpc(ctx, deps, rawConf.ResourceName(), conf, logger)
}

func NewLoggingIpc(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (resource.Resource, error) {
	cancelCtx, cancelFunc := context.WithCancel(ctx)

	s := &testingWindowsIpcLoggingIpc{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	// IPC server used by helpers to post a message we log.
	StartIPCServer(logger)

	// 1) Ensure desktop shortcut exists.
	if err := s.ensureDesktopShortcutOnConstruct(); err != nil {
		s.logger.Errorf("failed to create desktop shortcut on construct: %v", err)
	}

	// 2) Ensure tray helper is configured + started.
	//    - Create a "Common Startup" shortcut so it WILL appear for a logged-in user session.
	//    - Also try to start it immediately (best-effort; if viam-server runs as LocalSystem, it won't be visible).
	if err := s.ensureTrayOnConstruct(); err != nil {
		s.logger.Errorf("failed to configure/start tray helper on construct: %v", err)
	}

	return s, nil
}

func (s *testingWindowsIpcLoggingIpc) Name() resource.Name { return s.name }

func (s *testingWindowsIpcLoggingIpc) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	raw, ok := cmd["command"]
	if !ok {
		return nil, fmt.Errorf("missing 'command'")
	}
	command, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("'command' must be a string")
	}

	switch command {
	case "log":
		msg, _ := cmd["message"].(string)
		if msg == "" {
			msg = "(empty message)"
		}
		s.logger.Infof("DoCommand log: %s", msg)
		return map[string]interface{}{"ok": true}, nil

	case "create_shortcut":
		shortcutName, _ := cmd["shortcut_name"].(string)
		if shortcutName == "" {
			shortcutName = "Viam Logging IPC"
		}
		message, _ := cmd["message"].(string)
		if message == "" {
			message = "hello from desktop shortcut"
		}

		shortcutPath, helperExe, args, workDir, err := s.buildDesktopShortcutParts(shortcutName, message)
		if err != nil {
			return nil, err
		}

		if err := createWindowsShortcut(shortcutPath, helperExe, args, workDir); err != nil {
			s.logger.Errorf("create_shortcut failed: %v", err)
			return nil, err
		}

		s.logger.Infof("Created desktop shortcut: %s -> %s %s", shortcutPath, helperExe, args)
		return map[string]interface{}{
			"ok":       true,
			"shortcut": shortcutPath,
			"exe":      helperExe,
		}, nil

	default:
		return nil, fmt.Errorf("unknown command: %s", command)
	}
}

func (s *testingWindowsIpcLoggingIpc) Close(context.Context) error {
	s.cancelFunc()

	// Best-effort: stop any tray process we started directly.
	if s.trayCmd != nil && s.trayCmd.Process != nil {
		_ = s.trayCmd.Process.Kill()
		_, _ = s.trayCmd.Process.Wait()
	}

	return nil
}

// ------------------------
// Desktop shortcut
// ------------------------

func (s *testingWindowsIpcLoggingIpc) ensureDesktopShortcutOnConstruct() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	shortcutName := "Viam Logging IPC"
	message := "hello from desktop shortcut"

	if s.cfg != nil {
		if strings.TrimSpace(s.cfg.ShortcutName) != "" {
			shortcutName = strings.TrimSpace(s.cfg.ShortcutName)
		}
		if strings.TrimSpace(s.cfg.Message) != "" {
			message = strings.TrimSpace(s.cfg.Message)
		}
	}

	shortcutPath, helperExe, args, workDir, err := s.buildDesktopShortcutParts(shortcutName, message)
	if err != nil {
		return err
	}

	if err := createWindowsShortcut(shortcutPath, helperExe, args, workDir); err != nil {
		return err
	}

	s.logger.Infof("Created desktop shortcut on construct: %s -> %s %s", shortcutPath, helperExe, args)
	return nil
}

func (s *testingWindowsIpcLoggingIpc) buildDesktopShortcutParts(shortcutName, message string) (shortcutPath, helperExe, args, workDir string, err error) {
	desk := publicDesktopDir()
	shortcutPath = filepath.Join(desk, shortcutName+".lnk")

	helperExe, workDir, err = s.findSiblingExe("desktop-helper")
	if err != nil {
		return "", "", "", "", err
	}

	endpoint := s.effectiveEndpoint()
	args = fmt.Sprintf(`-endpoint "%s" -msg "%s"`, endpoint, message)
	return shortcutPath, helperExe, args, workDir, nil
}

// ------------------------
// Tray helper
// ------------------------

func (s *testingWindowsIpcLoggingIpc) ensureTrayOnConstruct() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	trayExe, workDir, err := s.findSiblingExe("tray-helper")
	if err != nil {
		return err
	}

	endpoint := s.effectiveEndpoint()

	// A) Ensure it will show up for an actual logged-in user:
	// Put a shortcut in the common Startup folder so it runs on user logon (interactive session).
	startup := commonStartupDir()
	startupShortcut := filepath.Join(startup, "Viam Logging IPC Tray.lnk")
	startupArgs := fmt.Sprintf(`-endpoint "%s"`, endpoint)
	if err := createWindowsShortcut(startupShortcut, trayExe, startupArgs, workDir); err != nil {
		return fmt.Errorf("failed creating startup shortcut for tray helper: %w", err)
	}
	s.logger.Infof("Created startup shortcut for tray helper: %s -> %s %s", startupShortcut, trayExe, startupArgs)

	// B) Best-effort: try to start it immediately as a child process.
	// NOTE: If viam-server is running as LocalSystem (Session 0), the tray icon will NOT be visible to the user.
	cmd := exec.CommandContext(s.cancelCtx, trayExe, "-endpoint", endpoint)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		// Don’t fail the module if this doesn’t start.
		s.logger.Warnf("tray-helper did not start immediately (startup shortcut still created): %v", err)
		return nil
	}

	s.trayCmd = cmd
	s.logger.Infof("Started tray helper (best effort): %s (pid=%d)", trayExe, cmd.Process.Pid)
	return nil
}

// ------------------------
// Helpers
// ------------------------

func (s *testingWindowsIpcLoggingIpc) effectiveEndpoint() string {
	endpoint := "http://127.0.0.1:17831/log"
	if s.cfg != nil && strings.TrimSpace(s.cfg.Endpoint) != "" {
		endpoint = strings.TrimSpace(s.cfg.Endpoint)
	}
	return endpoint
}

// findSiblingExe finds an executable in the same directory as the module executable.
// Your module runs with working dir = bin/, so this expects:
//
//	bin/desktop-helper.exe
//	bin/tray-helper.exe
func (s *testingWindowsIpcLoggingIpc) findSiblingExe(base string) (exePath string, workDir string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	workDir = filepath.Dir(exe)

	name := base
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}

	exePath = filepath.Join(workDir, name)
	if _, err := os.Stat(exePath); err != nil {
		return "", "", fmt.Errorf("%s not found next to module executable: %s (err=%v)", name, exePath, err)
	}

	return exePath, workDir, nil
}

func publicDesktopDir() string {
	// Prefer %PUBLIC%\Desktop if available.
	pub := os.Getenv("PUBLIC")
	if pub != "" {
		return filepath.Join(pub, "Desktop")
	}
	return `C:\Users\Public\Desktop`
}

func commonStartupDir() string {
	// Common Startup folder (runs for any user at logon):
	// C:\ProgramData\Microsoft\Windows\Start Menu\Programs\Startup
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	return filepath.Join(progData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
}
