# 观测模式

观测模式将浏览器的网络请求和响应以 JSONL 格式实时输出到 stdout，适合与 AI agent 或其他工具配合使用。

## 基本用法

```bash
twist --launch --observe -u https://example.com
```

无 `--observe-count` 或 `--observe-duration` 时，观测持续运行直到 Ctrl+C。

## 输出格式

每行一个 JSON 对象。每个 HTTP 请求产生两个事件：

### 请求事件

```json
{
  "type": "request",
  "requestId": "123.456",
  "url": "https://api.example.com/users",
  "method": "GET",
  "resourceType": "XHR",
  "requestHeaders": {
    "Accept": "application/json",
    "Authorization": "Bearer xxx"
  }
}
```

POST/PUT 请求还包含 `postData` 字段：

```json
{
  "type": "request",
  "requestId": "123.457",
  "url": "https://api.example.com/users",
  "method": "POST",
  "resourceType": "XHR",
  "requestHeaders": {"Content-Type": "application/json"},
  "postData": "{\"name\":\"test\"}"
}
```

### 成功响应事件

```json
{
  "type": "response",
  "requestId": "123.456",
  "url": "https://api.example.com/users",
  "resourceType": "XHR",
  "statusCode": 200,
  "statusText": "OK",
  "responseHeaders": {
    "Content-Type": "application/json",
    "Content-Length": "1234"
  },
  "body": "[{\"id\":1,\"name\":\"Alice\"}]",
  "bodyTruncated": false,
  "bodySize": 27
}
```

### 失败响应事件

```json
{
  "type": "response",
  "requestId": "123.458",
  "url": "https://bad.example.com/api",
  "errorReason": "NameNotResolved"
}
```

## 字段说明

| 字段 | 类型 | 出现于 | 说明 |
|------|------|--------|------|
| `type` | string | 始终 | `request` 或 `response` |
| `requestId` | string | 始终 | CDP 请求 ID，同一请求的 request 和 response 事件共享 |
| `url` | string | 始终 | 请求 URL |
| `resourceType` | string | 始终 | 资源类型：`XHR`、`Fetch`、`Document`、`Script`、`Stylesheet`、`Image` 等 |
| `method` | string | 请求 | HTTP 方法 |
| `requestHeaders` | object | 请求 | 请求头键值对 |
| `postData` | string | 请求 | POST/PUT 请求体 |
| `statusCode` | int | 成功响应 | HTTP 状态码 |
| `statusText` | string | 成功响应 | 状态文本 |
| `responseHeaders` | object | 成功响应 | 响应头键值对 |
| `body` | string | 成功响应 | 响应体（默认截断 4KB） |
| `bodyTruncated` | bool | 成功响应 | 响应体是否被截断 |
| `bodySize` | int | 成功响应 | 原始响应体大小（字节） |
| `errorReason` | string | 失败响应 | CDP 错误原因（如 `NameNotResolved`、`ConnectionRefused`） |

## 退出策略

### 计数退出

```bash
twist --launch --observe --observe-count 20 -u https://example.com
```

输出 20 个匹配事件后自动退出。注意每个 HTTP 请求产生 2 个事件（request + response）。

### 超时退出

```bash
twist --launch --observe --observe-duration 30s -u https://example.com
```

支持的单位：`s`（秒）、`m`（分）、`h`（时）。如 `90s`、`5m`、`1h`。

### 同时使用

```bash
twist --launch --observe --observe-count 20 --observe-duration 30s
```

任一条件触发即退出。

## 过滤

### URL 过滤

```bash
# 只看 URL 包含 "api" 或 "analytics" 的请求
twist --observe --observe-filter url=api,analytics
```

子串匹配，逗号分隔多个值表示任一匹配。

### 资源类型过滤

```bash
# 只看 XHR 和 Fetch 请求
twist --observe --observe-filter type=xhr,fetch
```

大小写不敏感。可用类型：`xhr`、`fetch`、`document`、`script`、`stylesheet`、`image`、`media`、`font`、`other`。

### 组合过滤

```bash
# 只看 API 中的 XHR 请求（多个 filter 之间 AND）
twist --observe --observe-filter url=api --observe-filter type=xhr
```

## 响应体

默认截断 4KB，字段 `bodyTruncated: true` 表示被截断，`bodySize` 表示原始大小。

```bash
# 获取完整响应体
twist --observe --observe-full-body
```

## AI Agent 集成

```bash
# AI 观察 50 个 API 请求后分析
twist --launch --observe --observe-filter url=api --observe-count 50 2>/dev/null | tee requests.jsonl

# AI 逐行处理
twist --launch --observe --observe-filter type=xhr 2>/dev/null | while read line; do
    url=$(echo "$line" | jq -r '.url // empty')
    [ -n "$url" ] && echo "Request: $url"
done
```

stdout 输出 JSONL，stderr 输出日志，AI agent 只需读取 stdout。
