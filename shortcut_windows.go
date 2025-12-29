//go:build windows

package testingwindowsipc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func desktopDir() (string, error) {
	// Visible for all users, works even when running as LocalSystem
	public := filepath.Join(os.Getenv("PUBLIC"), "Desktop") // usually C:\Users\Public\Desktop
	if public != `\Desktop` {                               // env var missing guard
		if st, err := os.Stat(public); err == nil && st.IsDir() {
			return public, nil
		}
	}

	// fallback to existing behavior
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "OneDrive", "Desktop"),
	}
	for _, d := range candidates {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d, nil
		}
	}
	return filepath.Join(home, "Desktop"), nil
}

func createWindowsShortcut(shortcutPath, targetExe, args, workDir string) error {
	ps := fmt.Sprintf(`$WshShell = New-Object -ComObject WScript.Shell;
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

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shortcut create failed: %v: %s", err, string(out))
	}
	return nil
}

func escapePSSingle(s string) string {
	// single-quoted PowerShell string: escape ' by doubling it
	r := ""
	for _, ch := range s {
		if ch == '\'' {
			r += "''"
		} else {
			r += string(ch)
		}
	}
	return r
}
