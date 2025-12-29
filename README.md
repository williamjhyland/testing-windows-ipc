# Module testing-windows-ipc 

A Windows-focused Viam module that demonstrates a simple “local UX → Viam logs” workflow:

- The module hosts a lightweight local HTTP server on 127.0.0.1:17831.
- A Desktop helper executable shows an interactive Windows MessageBox and POSTs a message to the local server.
- A System tray helper executable runs in the logged-in user’s session (tray icon) and can trigger the same “prompt + POST” flow.
- On module start, the service can create the desktop shortcut and start the tray helper (best effort).

This pattern is useful when you want an operator-facing UI surface (desktop / tray) that can “signal” a headless Viam process running as a service.
## What this module demonstrates

- Running an HTTP endpoint from inside a Viam module process.
- Creating Windows shortcuts (.lnk) programmatically.
- Bridging a user-interactive process (MessageBox) to a headless service via localhost.
- Running a tray helper as a separate executable so the icon appears in the user session (not LocalSystem).

## Binaries packaged with the module

The module expects these executables to live next to the module binary under the module root (often bin/):
- testing-windows-ipc.exe (module executable)
- desktop-helper.exe (desktop shortcut target)
- tray-helper.exe (system tray app)

### Configuration
The following attribute template can be used to configure this model:

```json
{
  "services": [
    {
      "name": "generic-1",
      "api": "rdk:service:generic",
      "model": "bill:testing-windows-ipc:logging-ipc",
      "attributes": {}
    }
  ],
  "modules": [
    {
      "type": "local",
      "name": "local-shortcut-gen",
      "executable_path": "C:\\Users\\Viam\\Viam-Modules\\testing-windows-ipc\\bin\\testing-windows-ipc.exe"
    }
  ]
}
```

## Local IPC server

The module starts an HTTP server listening on:

http://127.0.0.1:17831

Endpoints

GET /healthz → returns 200 ok

POST /log (or POST /notify) → logs the message to the Viam module logger