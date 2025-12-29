// cmd/tray-helper/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
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
	_, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	return err
}

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:17831/log", "IPC endpoint")
	msg := flag.String("msg", "hello from tray", "message to send")
	flag.Parse()

	systray.Run(func() {
		systray.SetTitle("Viam Logger")
		systray.SetTooltip("Viam Logger")

		mSend := systray.AddMenuItem("Send log…", "Send a log message")
		mQuit := systray.AddMenuItem("Quit", "Quit")

		go func() {
			for {
				select {
				case <-mSend.ClickedCh:
					choice := messageBox("Viam Logger", "Send this message to Viam logs?\n\n"+*msg, mbYesNo|mbIconInfo)
					if choice == idYes {
						if err := postToIPC(*endpoint, *msg); err != nil {
							messageBox("Viam Logger", "Failed:\n\n"+err.Error(), mbOk|mbIconErr)
						} else {
							messageBox("Viam Logger", "Sent.", mbOk|mbIconInfo)
						}
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
