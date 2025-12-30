//go:build windows

package testingwindowsipc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// desktopDir returns a desktop location that is valid when running
// as a Windows service (LocalSystem).
//
// We intentionally ONLY use the Public Desktop. Services must not
// guess interactive user home directories.
func desktopDir() (string, error) {
	pub := os.Getenv("PUBLIC")
	if pub == "" {
		return "", fmt.Errorf("PUBLIC environment variable not set")
	}

	publicDesktop := filepath.Join(pub, "Desktop")
	if st, err := os.Stat(publicDesktop); err == nil && st.IsDir() {
		return publicDesktop, nil
	}

	return "", fmt.Errorf("public desktop not found at %q", publicDesktop)
}

// moduleExeDir returns the directory containing the currently
// running module executable.
func moduleExeDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// createWindowsShortcut creates a .lnk file using PowerShell COM automation.
func createWindowsShortcut(shortcutPath, targetExe, args, workDir string) error {
	ps := fmt.Sprintf(
		`$WshShell = New-Object -ComObject WScript.Shell;
$Shortcut = $WshShell.CreateShortcut('%s');
$Shortcut.TargetPath = '%s';
$Shortcut.Arguments = '%s';
$Shortcut.WorkingDirectory = '%s';
$Shortcut.Save();`,
		escapePSSingle(shortcutPath),
		escapePSSingle(targetExe),
		escapePSSingle(args),
		escapePSSingle(workDir),
	)

	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", ps,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shortcut create failed: %v: %s", err, string(out))
	}

	return nil
}

// escapePSSingle escapes a string for use inside a single-quoted
// PowerShell string literal.
func escapePSSingle(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\'' {
			out = append(out, '\'', '\'')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
