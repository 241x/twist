# Twist 规则配置文件格式

Twist 使用 JSON 格式的规则配置文件来定义网络请求/响应的拦截和修改行为。

---

## 配置文件结构

### 根对象

```json
{
  "id": "twist-20260611-abc123",
  "name": "配置名称",
  "version": "1.0",
  "description": "配置说明",
  "settings": {},
  "rules": []
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 配置唯一标识，格式：`twist-YYYYMMDD-随机6位` |
| `name` | string | 是 | 配置名称 |
| `version` | string | 是 | 配置版本（当前 `1.0`） |
| `description` | string | 否 | 配置描述 |
| `settings` | object | 否 | 全局设置（预留） |
| `rules` | Rule[] | 是 | 规则列表 |

---

### 规则对象（Rule）

```json
{
  "id": "rule-001",
  "name": "规则名称",
  "enabled": true,
  "priority": 0,
  "stage": "request",
  "match": {
    "allOf": [],
    "anyOf": []
  },
  "actions": []
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 规则唯一标识，格式：`rule-XXX` |
| `name` | string | 是 | 规则名称 |
| `enabled` | boolean | 是 | 是否启用 |
| `priority` | number | 是 | 优先级，数值越大越先执行 |
| `stage` | string | 是 | 生命周期阶段：`request` 或 `response` |
| `match` | Match | 是 | 匹配条件 |
| `actions` | Action[] | 是 | 执行行为列表 |

---

## 生命周期阶段

### `request` — 请求阶段

在请求发送到服务器之前拦截，可修改 URL、方法、请求头、请求体，也可通过 `block` 行为阻止请求。

### `response` — 响应阶段

在响应返回到浏览器之前拦截，可修改状态码、响应头、响应体。不能阻止请求（请求已发送）。

---

## 匹配条件（Match）

匹配条件通过 `allOf`（AND 逻辑）和 `anyOf`（OR 逻辑）组合。两者同时存在时，`allOf` 全部满足 **且** `anyOf` 至少满足一个。

```json
{
  "match": {
    "allOf": [
      {"type": "urlContains", "value": "/api/"},
      {"type": "method", "values": ["POST"]}
    ],
    "anyOf": [
      {"type": "headerExists", "name": "Authorization"},
      {"type": "cookieExists", "name": "token"}
    ]
  }
}
```

---

### URL 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `urlEquals` | `value` (string) | URL 精确匹配 |
| `urlPrefix` | `value` (string) | URL 前缀匹配 |
| `urlSuffix` | `value` (string) | URL 后缀匹配 |
| `urlContains` | `value` (string) | URL 包含指定字符串 |
| `urlRegex` | `pattern` (string) | URL 正则匹配 |

**示例：**

```json
{"type": "urlEquals",  "value": "https://example.com/api/user"}
{"type": "urlPrefix",  "value": "https://example.com/api/"}
{"type": "urlSuffix",  "value": ".json"}
{"type": "urlContains", "value": "/api/user"}
{"type": "urlRegex",   "pattern": "^https://example\\.com/api/(user|order)/\\d+$"}
```

---

### HTTP 属性条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `method` | `values` (string[]) | HTTP 方法：`GET` `POST` `PUT` `DELETE` `PATCH` `HEAD` `OPTIONS` |
| `resourceType` | `values` (string[]) | 资源类型：`document` `script` `stylesheet` `image` `media` `font` `xhr` `fetch` `websocket` `other` |

**示例：**

```json
{"type": "method",       "values": ["GET", "POST"]}
{"type": "resourceType", "values": ["xhr", "fetch"]}
```

---

### Header 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `headerExists` | `name` (string) | Header 存在 |
| `headerNotExists` | `name` (string) | Header 不存在 |
| `headerEquals` | `name` `value` (string) | Header 值精确匹配 |
| `headerContains` | `name` `value` (string) | Header 值包含匹配 |
| `headerRegex` | `name` `pattern` (string) | Header 值正则匹配 |

**示例：**

```json
{"type": "headerExists",   "name": "Authorization"}
{"type": "headerEquals",   "name": "Content-Type", "value": "application/json"}
{"type": "headerContains", "name": "User-Agent", "value": "Chrome"}
```

---

### Query 参数条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `queryExists` | `name` (string) | Query 参数存在 |
| `queryNotExists` | `name` (string) | Query 参数不存在 |
| `queryEquals` | `name` `value` (string) | Query 参数值精确匹配 |
| `queryContains` | `name` `value` (string) | Query 参数值包含匹配 |
| `queryRegex` | `name` `pattern` (string) | Query 参数值正则匹配 |

**示例：**

```json
{"type": "queryEquals", "name": "page", "value": "1"}
```

---

### Cookie 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `cookieExists` | `name` (string) | Cookie 存在 |
| `cookieNotExists` | `name` (string) | Cookie 不存在 |
| `cookieEquals` | `name` `value` (string) | Cookie 值精确匹配 |
| `cookieContains` | `name` `value` (string) | Cookie 值包含匹配 |
| `cookieRegex` | `name` `pattern` (string) | Cookie 值正则匹配 |

**示例：**

```json
{"type": "cookieExists", "name": "sessionId"}
```

---

### Body 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `bodyContains` | `value` (string) | Body 包含指定字符串 |
| `bodyRegex` | `pattern` (string) | Body 正则匹配 |
| `bodyJsonPath` | `path` `value` (string) | JSON Path 匹配 |

**示例：**

```json
{"type": "bodyContains", "value": "username"}
{"type": "bodyRegex",    "pattern": "\"userId\":\\s*\\d+"}
{"type": "bodyJsonPath", "path": "$.user.id", "value": "123"}
```

---

## 执行行为（Action）

### 请求阶段专用

| 类型 | 参数 | 说明 |
|------|------|------|
| `setUrl` | `value` (string) | 修改请求 URL |
| `setMethod` | `value` (string) | 修改 HTTP 方法 |
| `setQueryParam` | `name` `value` (any) | 设置查询参数 |
| `removeQueryParam` | `name` (string) | 移除查询参数 |
| `setCookie` | `name` `value` (any) | 设置 Cookie |
| `removeCookie` | `name` (string) | 移除 Cookie |
| `setFormField` | `name` `value` (any) | 设置表单字段 |
| `removeFormField` | `name` (string) | 移除表单字段 |
| `block` | `statusCode` `headers` `body` `bodyEncoding` | 拦截请求，返回自定义响应 |

**`block` 行为详解：**

```json
{
  "type": "block",
  "statusCode": 403,
  "headers": {"Content-Type": "application/json"},
  "body": "{\"error\": \"Blocked\"}",
  "bodyEncoding": "text"
}
```

`block` 是终结性行为，后续 action 不再执行。

---

### 响应阶段专用

| 类型 | 参数 | 说明 |
|------|------|------|
| `setStatus` | `value` (number) | 修改响应状态码 |

---

### 通用行为（请求/响应均可）

| 类型 | 参数 | 说明 |
|------|------|------|
| `setHeader` | `name` `value` (any) | 设置头部 |
| `removeHeader` | `name` (string) | 移除头部 |
| `setBody` | `value` `encoding` | 替换 Body 内容 |
| `replaceBodyText` | `search` `replace` `replaceAll` | 字符串替换 Body |
| `patchBodyJson` | `patches` (Patch[]) | JSON Patch 修改 Body（RFC 6902） |

**`setBody` 示例：**

```json
{"type": "setBody", "value": "{\"code\":0,\"data\":{}}", "encoding": "text"}
```

`encoding` 可选 `text`（默认）或 `base64`。

**`replaceBodyText` 示例：**

```json
{"type": "replaceBodyText", "search": "old", "replace": "new", "replaceAll": true}
```

**`patchBodyJson` 示例：**

```json
{
  "type": "patchBodyJson",
  "patches": [
    {"op": "replace", "path": "/user/name", "value": "newName"},
    {"op": "add",     "path": "/user/age",  "value": 25}
  ]
}
```

支持的 JSON Patch 操作：`add` `remove` `replace` `move` `copy` `test`（遵循 RFC 6902）。

---

## 完整配置示例

```json
{
  "id": "twist-20260611-demo01",
  "name": "演示配置",
  "version": "1.0",
  "description": "包含常见规则的演示配置",
  "settings": {},
  "rules": [
    {
      "id": "rule-001",
      "name": "Mock 用户信息接口",
      "enabled": true,
      "priority": 10,
      "stage": "response",
      "match": {
        "allOf": [
          {"type": "urlContains", "value": "/api/user/info"}
        ]
      },
      "actions": [
        {
          "type": "setBody",
          "value": "{\"code\":0,\"data\":{\"id\":1,\"name\":\"测试用户\",\"email\":\"test@example.com\"}}",
          "encoding": "text"
        }
      ]
    },
    {
      "id": "rule-002",
      "name": "全局 CORS 处理",
      "enabled": true,
      "priority": 5,
      "stage": "response",
      "match": {},
      "actions": [
        {"type": "setHeader", "name": "Access-Control-Allow-Origin", "value": "*"},
        {"type": "setHeader", "name": "Access-Control-Allow-Methods", "value": "GET, POST, PUT, DELETE, PATCH, OPTIONS"},
        {"type": "setHeader", "name": "Access-Control-Allow-Headers", "value": "Content-Type, Authorization"}
      ]
    },
    {
      "id": "rule-003",
      "name": "阻止追踪请求",
      "enabled": true,
      "priority": 0,
      "stage": "request",
      "match": {
        "anyOf": [
          {"type": "urlContains", "value": "google-analytics.com"},
          {"type": "urlContains", "value": "analytics"}
        ]
      },
      "actions": [
        {"type": "block", "statusCode": 204}
      ]
    }
  ]
}
```

---

## 规则执行流程

```
请求到达 → 按 priority 降序匹配请求阶段规则 → 执行匹配的 actions → 发送请求
                                                                          ↓
响应到达 → 按 priority 降序匹配响应阶段规则 → 执行匹配的 actions → 返回浏览器
```

- 同一阶段内，规则按 `priority` 降序执行
- 同一规则内，actions 按数组顺序执行
- `block` 行为终止当前请求流程，不再执行后续 actions
