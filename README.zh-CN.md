# twist

> 通过 CDP 协议实时观测和修改浏览器网络请求/响应的命令行工具。

[English](README.md)

## 快速开始

```bash
go install github.com/241x/twist@latest

# 观测网络请求（JSONL 输出到 stdout）
twist --launch --observe -u https://example.com

# 按条件过滤观测
twist --launch --observe --observe-filter type=xhr --observe-count 20

# 启动 Chrome 并加载修改规则
twist --launch -c rules.json -u https://example.com

# 连接已有浏览器
twist -c rules.json

# 列出可用标签页
twist --list-targets
```

## 功能

- **网络观测** — 实时监控网络请求，JSONL 输出，支持过滤和退出条件
- **拦截修改** — 阻止请求、模拟响应、重写请求头/URL/请求体
- **25 种匹配条件** — URL、方法、资源类型、请求头、Query 参数、Cookie、请求体（正则 + JSON Path）
- **17 种执行行为** — block、setHeader、removeHeader、setUrl、setMethod、setQueryParam、setCookie、setFormField、setStatus、setBody、appendBody、replaceBodyText、patchBodyJson（RFC 6902）、replaceElement
- **请求/响应阶段** — 在服务器收到前修改请求，或在浏览器收到前修改响应
- **自动启动浏览器** — 查找 Windows/macOS/Linux 上的 Chrome/Chromium/Edge
- **管道支持** — `cat rules.json | twist`
- **并发处理** — 多 worker 池，超时与 panic 恢复

## 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-H, --host` | `127.0.0.1` | 浏览器 CDP 地址 |
| `-p, --port` | `9222` | CDP 端口（`--launch` 时作为调试端口） |
| `--launch` | `false` | 自动启动浏览器 |
| `--launch-browser` | `chrome` | `chrome`、`chromium`、`edge` |
| `--launch-args` | — | 浏览器额外参数（可重复） |
| `-u, --url` | — | 打开的网址 |
| `-c, --config` | — | 规则配置文件路径 |
| `-t, --target` | — | 附加到指定标签页 ID |
| `--list-targets` | `false` | 列出标签页后退出 |
| `--observe` | `false` | 观测模式：输出网络事件 JSONL 到 stdout |
| `--observe-count` | `0` | 观测 N 个匹配事件后退出（0 = 不限） |
| `--observe-duration` | — | 观测一段时间后退出（如 `30s`、`5m`） |
| `--observe-filter` | — | 过滤事件（可重复，格式：`key=值1,值2`） |
| `--observe-full-body` | `false` | 输出完整响应体（默认截断 4KB） |
| `-v, --verbose` | `false` | 详细调试日志 |
| `--timeout` | `15` | CDP 连接超时（秒） |

## 观测模式

```bash
# 观测所有请求和响应
twist --launch --observe -u https://example.com

# 只看 XHR 请求，20 个事件后退出
twist --launch --observe --observe-filter type=xhr --observe-count 20

# 只看 API 请求，30 秒后退出
twist --launch --observe --observe-filter url=api --observe-duration 30s

# 输出完整响应体
twist --launch --observe --observe-full-body
```

输出格式（JSONL，一行一个事件）：

```json
{"type":"request","requestId":"123","url":"https://api.x.com/users","method":"GET","resourceType":"XHR","requestHeaders":{...}}
{"type":"response","requestId":"123","url":"https://api.x.com/users","statusCode":200,"statusText":"OK","responseHeaders":{...},"body":"[...]","bodySize":1234}
{"type":"response","requestId":"456","url":"https://bad.com/api","errorReason":"NameNotResolved"}
```

过滤字段：`url`（子串匹配）、`type`（资源类型，大小写不敏感）。

详见 [观测模式文档](docs/05-observe-mode.md)。

## 示例配置

```json
{
  "id": "twist-20260611-demo01",
  "name": "演示",
  "version": "1.0",
  "rules": [
    {
      "id": "rule-001",
      "name": "阻止统计请求",
      "enabled": true,
      "priority": 10,
      "stage": "request",
      "match": { "allOf": [{"type": "urlContains", "value": "analytics"}] },
      "actions": [{"type": "block", "statusCode": 204}]
    },
    {
      "id": "rule-002",
      "name": "模拟接口",
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

## 文档

- [命令行参数详解](docs/01-cli-usage.md)
- [观测模式](docs/05-observe-mode.md)
- [规则配置文件格式](docs/02-config-format.md)
- [浏览器与 CDP 交互](docs/03-browser-cdp.md)
- [进阶说明](docs/04-advanced.md)

## 环境要求

- Go 1.26+
- Chrome / Chromium / Edge（自动启动时），或任意已开启 `--remote-debugging-port` 的浏览器

## 许可证

MIT
