# Changelog

## v1.1.0

### Added

- `--observe` mode: real-time network monitoring with JSONL output to stdout
- `--observe-count N`: exit after N matching events
- `--observe-duration D`: exit after duration (e.g. `30s`, `5m`)
- `--observe-filter key=val1,val2`: filter events by URL substring or resource type
- `--observe-full-body`: include full response body (default truncated to 4KB)
- Failed request tracking: `errorReason` field in response events for network errors
- Mode validation: `--observe` is mutually exclusive with `--config` and `--list-targets`
- Observe-only flags (`--observe-count`, etc.) error without `--observe`

## v1.0.0

### Initial release

- Intercept and modify browser network requests/responses via CDP
- 25 match conditions and 17 actions
- Auto-launch browser (Chrome/Chromium/Edge on Windows/macOS/Linux)
- Rule configuration via file, stdin, or default paths
- Multi-worker pool with timeout and panic recovery
