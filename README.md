# twist

> A CLI tool that intercepts and modifies browser network requests/responses in real time via Chrome DevTools Protocol (CDP).

[中文文档](README.zh-CN.md)

## Quick Start

```bash
go install github.com/241x/twist/cmd/twist@latest

# Launch Chrome and intercept with rules
twist --launch -c rules.json -u https://example.com

# Connect to existing browser
twist -c rules.json

# List available tabs
twist --list-targets

# Pipe rules via stdin
cat rules.json | twist --launch
```

## Features

- **Intercept & modify** — block requests, mock responses, rewrite headers/URLs/body
- **25 match conditions** — URL, method, resource type, headers, query params, cookies, request body (regex + JSON Path)
- **16 actions** — block, setHeader, removeHeader, setUrl, setMethod, setQueryParam, setCookie, setFormField, setStatus, setBody, replaceBodyText, patchBodyJson (RFC 6902)
- **Request & response stage** — modify before server receives, or after browser gets response
- **Auto-launch browser** — finds Chrome/Chromium/Edge on Windows/macOS/Linux
- **Pipe support** — `cat rules.json | twist`
- **Concurrent** — multi-worker pool with timeout and panic recovery

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `-H, --host` | `127.0.0.1` | Browser CDP address |
| `-p, --port` | `9222` | CDP port (debug port with `--launch`) |
| `--launch` | `false` | Auto-launch a new browser |
| `--launch-browser` | `chrome` | `chrome`, `chromium`, `edge` |
| `--launch-args` | — | Extra browser args (repeatable) |
| `-u, --url` | — | URL to open |
| `-c, --config` | — | Rules config file path |
| `-t, --target` | — | Attach to specific tab ID |
| `--list-targets` | `false` | List tabs and exit |
| `-v, --verbose` | `false` | Verbose debug logging |
| `--timeout` | `30` | CDP connection timeout (seconds) |

## Example Config

```json
{
  "id": "twist-20260611-demo01",
  "name": "Demo",
  "version": "1.0",
  "rules": [
    {
      "id": "rule-001",
      "name": "Block analytics",
      "enabled": true,
      "priority": 10,
      "stage": "request",
      "match": { "allOf": [{"type": "urlContains", "value": "analytics"}] },
      "actions": [{"type": "block", "statusCode": 204}]
    },
    {
      "id": "rule-002",
      "name": "Mock API",
      "enabled": true,
      "priority": 5,
      "stage": "response",
      "match": { "allOf": [{"type": "urlPrefix", "value": "https://api.example.com/"}] },
      "actions": [
        {"type": "setHeader", "name": "Access-Control-Allow-Origin", "value": "*"},
        {"type": "setBody", "value": "{\"ok\":true}"}
      ]
    }
  ]
}
```

## Documentation

- [CLI Usage & Parameters](docs/01-cli-usage.md)
- [Rule Configuration Format](docs/02-config-format.md)
- [Browser & CDP Interaction](docs/03-browser-cdp.md)
- [Advanced Topics](docs/04-advanced.md)

## Requirements

- Go 1.26+
- Chrome / Chromium / Edge (for auto-launch), or any browser with `--remote-debugging-port` enabled

## License

MIT
