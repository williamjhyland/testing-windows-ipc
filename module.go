package testingwindowsipc

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	ShortcutName string `json:"shortcut_name,omitempty"`
	Message      string `json:"message,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
}

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

	trayCmd *exec.Cmd
}

func newTestingWindowsIpcLoggingIpc(
	ctx context.Context,
	deps resource.Dependencies,
	rawConf resource.Config,
	logger logging.Logger,
) (resource.Resource, error) {

	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}
	return NewLoggingIpc(ctx, deps, rawConf.ResourceName(), conf, logger)
}

func NewLoggingIpc(
	ctx context.Context,
	deps resource.Dependencies,
	name resource.Name,
	conf *Config,
	logger logging.Logger,
) (resource.Resource, error) {

	cancelCtx, cancelFunc := context.WithCancel(ctx)

	s := &testingWindowsIpcLoggingIpc{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	// IPC server (module only)
	StartIPCServer(logger)

	if runtime.GOOS == "windows" {
		if err := s.ensureStableHelpers(); err != nil {
			s.logger.Errorf("failed to ensure stable helpers: %v", err)
		}
	}

	if err := s.ensureDesktopShortcutOnConstruct(); err != nil {
		s.logger.Errorf("failed to create desktop shortcut: %v", err)
	}

	if err := s.ensureTrayOnConstruct(); err != nil {
		s.logger.Errorf("failed to configure tray helper: %v", err)
	}

	return s, nil
}

func (s *testingWindowsIpcLoggingIpc) Name() resource.Name { return s.name }

func (s *testingWindowsIpcLoggingIpc) Close(context.Context) error {
	s.cancelFunc()

	if s.trayCmd != nil && s.trayCmd.Process != nil {
		_ = s.trayCmd.Process.Kill()
		_, _ = s.trayCmd.Process.Wait()
	}

	return nil
}

func (s *testingWindowsIpcLoggingIpc) DoCommand(
	ctx context.Context,
	cmd map[string]interface{},
) (map[string]interface{}, error) {

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

		shortcutPath := filepath.Join(
			publicDesktopDir(),
			shortcutName+".lnk",
		)

		workDir := stableHelperDir()
		helperExe := filepath.Join(workDir, "desktop-helper.exe")

		args := fmt.Sprintf(
			`-endpoint "%s" -msg "%s"`,
			s.effectiveEndpoint(),
			message,
		)

		if err := createWindowsShortcut(
			shortcutPath,
			helperExe,
			args,
			workDir,
		); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"ok":       true,
			"shortcut": shortcutPath,
			"exe":      helperExe,
		}, nil

	default:
		return nil, fmt.Errorf("unknown command: %s", command)
	}
}

//
// --------------------
// Stable helper install
// --------------------
//

func (s *testingWindowsIpcLoggingIpc) ensureStableHelpers() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	moduleBin := filepath.Dir(exe)

	stableDir := stableHelperDir()
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		return err
	}

	for _, name := range []string{"desktop-helper.exe", "tray-helper.exe"} {
		src := filepath.Join(moduleBin, name)
		dst := filepath.Join(stableDir, name)
		if err := copyReplace(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}

	s.logger.Infof("Stable helpers refreshed in %s", stableDir)
	return nil
}

func copyReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, dst)
}

//
// --------------------
// Desktop shortcut
// --------------------
//

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

	shortcutPath := filepath.Join(publicDesktopDir(), shortcutName+".lnk")
	helperExe := filepath.Join(stableHelperDir(), "desktop-helper.exe")
	workDir := stableHelperDir()

	args := fmt.Sprintf(`-endpoint "%s" -msg "%s"`, s.effectiveEndpoint(), message)

	return createWindowsShortcut(shortcutPath, helperExe, args, workDir)
}

//
// --------------------
// Tray helper
// --------------------
//

func (s *testingWindowsIpcLoggingIpc) ensureTrayOnConstruct() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	workDir := stableHelperDir()
	trayExe := filepath.Join(workDir, "tray-helper.exe")
	endpoint := s.effectiveEndpoint()

	startupShortcut := filepath.Join(
		commonStartupDir(),
		"Viam Logging IPC Tray.lnk",
	)

	if err := createWindowsShortcut(
		startupShortcut,
		trayExe,
		fmt.Sprintf(`-endpoint "%s"`, endpoint),
		workDir,
	); err != nil {
		return err
	}

	cmd := exec.CommandContext(s.cancelCtx, trayExe, "-endpoint", endpoint)
	cmd.Dir = workDir

	if err := cmd.Start(); err != nil {
		s.logger.Warnf("tray-helper did not start immediately: %v", err)
		return nil
	}

	s.trayCmd = cmd
	return nil
}

//
// --------------------
// Helpers
// --------------------
//

func (s *testingWindowsIpcLoggingIpc) effectiveEndpoint() string {
	endpoint := "http://127.0.0.1:17831/log"
	if s.cfg != nil && strings.TrimSpace(s.cfg.Endpoint) != "" {
		endpoint = strings.TrimSpace(s.cfg.Endpoint)
	}
	return endpoint
}

func stableHelperDir() string {
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	return filepath.Join(progData, "Viam", "testing-windows-ipc")
}

func publicDesktopDir() string {
	pub := os.Getenv("PUBLIC")
	if pub != "" {
		return filepath.Join(pub, "Desktop")
	}
	return `C:\Users\Public\Desktop`
}

func commonStartupDir() string {
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	return filepath.Join(
		progData,
		"Microsoft",
		"Windows",
		"Start Menu",
		"Programs",
		"Startup",
	)
}
