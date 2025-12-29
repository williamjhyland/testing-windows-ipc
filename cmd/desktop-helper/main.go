// cmd/desktop-helper/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOk       = 0x00000000
	mbYesNo    = 0x00000004
	mbIconInfo = 0x00000040
	mbIconErr  = 0x00000010
	idYes      = 6
)

func messageBox(title, text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

func postToIPC(endpoint, msg string) error {
	body, _ := json.Marshal(map[string]string{"msg": msg})

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		if len(b) == 0 {
			return fmt.Errorf("server returned %s", resp.Status)
		}
		return fmt.Errorf("server returned %s: %s", resp.Status, string(b))
	}

	return nil
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:17831/log", "IPC endpoint")
	msg := flag.String("msg", "hello from desktop shortcut", "message to send")
	flag.Parse()

	choice := messageBox("Viam Logger", "Send this message to Viam logs?\n\n"+*msg, mbYesNo|mbIconInfo)
	if choice == idYes {
		if err := postToIPC(*endpoint, *msg); err != nil {
			messageBox("Viam Logger", "Failed to send:\n\n"+err.Error(), mbOk|mbIconErr)
			os.Exit(1)
		}
		messageBox("Viam Logger", "Sent.", mbOk|mbIconInfo)
	}
}
