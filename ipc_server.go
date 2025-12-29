package testingwindowsipc

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.viam.com/rdk/logging"
)

var ipcOnce sync.Once

const ipcAddr = "127.0.0.1:17831"

func StartIPCServer(logger logging.Logger) {
	ipcOnce.Do(func() {
		mux := http.NewServeMux()

		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		handlePost := func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			var body struct {
				Message string `json:"message"`
				Msg     string `json:"msg"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			msg := strings.TrimSpace(body.Message)
			if msg == "" {
				msg = strings.TrimSpace(body.Msg)
			}
			if msg == "" {
				msg = "(empty message)"
			}

			logger.Infof("Desktop shortcut invoked: %s", msg)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("logged"))
		}

		// Support both routes.
		mux.HandleFunc("/notify", handlePost)
		mux.HandleFunc("/log", handlePost)

		srv := &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		ln, err := net.Listen("tcp", ipcAddr)
		if err != nil {
			logger.Errorf("IPC server failed to listen on %s: %v", ipcAddr, err)
			return
		}

		logger.Infof("IPC server listening on http://%s", ipcAddr)
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Errorf("IPC server error: %v", err)
			}
		}()
	})
}
