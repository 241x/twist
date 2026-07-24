# twist

> A CLI tool that observes and modifies browser network requests/responses in real time via Chrome DevTools Protocol (CDP).

[中文文档](README.zh-CN.md)

## Quick Start

```bash
go install github.com/241x/twist@latest

# Observe network requests (JSONL to stdout)
twist --launch --observe -u https://example.com

# Observe with filter and count limit
twist --launch --observe --observe-filter type=xhr --observe-count 20

# Intercept and modify with rules
twist --launch -c rules.json -u https://example.com

# Connect to existing browser
twist -c rules.json

# List available tabs
twist --list-targets
```

## Features

- **Observe** — real-time network monitoring with JSONL output, filters, and exit conditions
- **Intercept & modify** — block requests, mock responses, rewrite headers/URLs/body
- **25 match conditions** — URL, method, resource type, headers, query params, cookies, request body (regex + JSON Path)
- **17 actions** — block, setHeader, removeHeader, setUrl, setMethod, setQueryParam, setCookie, setFormField, setStatus, setBody, appendBody, replaceBodyText, patchBodyJson (RFC 6902), replaceElement
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
| `--observe` | `false` | Observe mode: output network events as JSONL to stdout |
| `--observe-count` | `0` | Exit after N matching events (0 = unlimited) |
| `--observe-duration` | — | Exit after duration (e.g. `30s`, `5m`) |
| `--observe-filter` | — | Filter events (repeatable, format: `key=val1,val2`) |
| `--observe-full-body` | `false` | Include full response body (default: truncated to 4KB) |
| `-v, --verbose` | `false` | Verbose debug logging |
| `--timeout` | `15` | CDP connection timeout (seconds) |

## Observe Mode

```bash
# Watch all requests and responses
twist --launch --observe -u https://example.com

# Watch 20 XHR events only
twist --launch --observe --observe-filter type=xhr --observe-count 20

# Watch API requests for 30 seconds
twist --launch --observe --observe-filter url=api --observe-duration 30s

# Watch with full response bodies
twist --launch --observe --observe-full-body
```

Output format (JSONL, one event per line):

```json
{"type":"request","requestId":"123","url":"https://api.x.com/users","method":"GET","resourceType":"XHR","requestHeaders":{...}}
{"type":"response","requestId":"123","url":"https://api.x.com/users","statusCode":200,"statusText":"OK","responseHeaders":{...},"body":"[...]","bodySize":1234}
{"type":"response","requestId":"456","url":"https://bad.com/api","errorReason":"NameNotResolved"}
```

Filter keys: `url` (substring match), `type` (resource type, case-insensitive).

See [Observe Mode](docs/05-observe-mode.md) for full documentation.

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
- [Observe Mode](docs/05-observe-mode.md)
- [Rule Configuration Format](docs/02-config-format.md)
- [Browser & CDP Interaction](docs/03-browser-cdp.md)
- [Advanced Topics](docs/04-advanced.md)

## Requirements

- Go 1.26+
- Chrome / Chromium / Edge (for auto-launch), or any browser with `--remote-debugging-port` enabled

## License

MIT
