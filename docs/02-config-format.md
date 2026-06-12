# 规则配置文件格式

Twist 使用 JSON 格式的规则配置文件。

## 根对象

```json
{
  "id": "twist-20260611-abc123",
  "name": "配置名称",
  "version": "1.0",
  "description": "配置说明",
  "rules": []
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 唯一标识，建议格式：`twist-YYYYMMDD-随机6位` |
| `name` | string | 是 | 配置名称 |
| `version` | string | 是 | 版本号，当前为 `1.0` |
| `description` | string | 否 | 描述信息 |
| `rules` | Rule[] | 是 | 规则列表，至少一条 |

## 规则对象

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
| `id` | string | 是 | 唯一标识，格式：`rule-XXX` |
| `name` | string | 是 | 规则名称 |
| `enabled` | boolean | 是 | 是否启用 |
| `priority` | number | 是 | 优先级，数值越大越先执行 |
| `stage` | string | 是 | 生命周期：`request`（请求阶段）或 `response`（响应阶段） |
| `match` | object | 是 | 匹配条件 |
| `actions` | Action[] | 是 | 执行行为列表，按数组顺序执行 |

## 生命周期阶段

### request（请求阶段）

在请求发送到服务器之前拦截。可修改 URL、方法、请求头、请求体，也可通过 `block` 阻止请求。

适用于：添加认证头、重写 API 地址、阻止追踪请求、修改 POST 参数。

### response（响应阶段）

在响应返回浏览器之前拦截。可修改状态码、响应头、响应体。不能阻止请求（请求已发送）。

适用于：Mock 接口数据、解决跨域问题、替换响应内容。

## 匹配条件

匹配条件通过 `allOf`（AND）和 `anyOf`（OR）组合：

- **allOf**：所有条件都必须满足
- **anyOf**：至少一个条件满足
- 两者同时存在时：allOf 全部满足 **且** anyOf 至少一个满足
- 两者均为空数组：匹配所有请求

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

### URL 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `urlEquals` | `value` | URL 精确匹配 |
| `urlPrefix` | `value` | URL 前缀匹配 |
| `urlSuffix` | `value` | URL 后缀匹配 |
| `urlContains` | `value` | URL 包含指定字符串 |
| `urlRegex` | `pattern` | URL 正则匹配 |

### HTTP 属性条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `method` | `values` (string[]) | HTTP 方法：GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS |
| `resourceType` | `values` (string[]) | 资源类型：`document`/`script`/`stylesheet`/`image`/`media`/`font`/`xhr`/`fetch`/`websocket`/`other` |

### Header 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `headerExists` | `name` | 请求头存在 |
| `headerNotExists` | `name` | 请求头不存在 |
| `headerEquals` | `name`, `value` | 请求头值精确匹配 |
| `headerContains` | `name`, `value` | 请求头值包含匹配 |
| `headerRegex` | `name`, `pattern` | 请求头值正则匹配 |

### Query 参数条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `queryExists` | `name` | Query 参数存在 |
| `queryNotExists` | `name` | Query 参数不存在 |
| `queryEquals` | `name`, `value` | Query 参数值精确匹配 |
| `queryContains` | `name`, `value` | Query 参数值包含匹配 |
| `queryRegex` | `name`, `pattern` | Query 参数值正则匹配 |

### Cookie 条件

| 类型 | 参数 | 说明 |
|------|------|------|
| `cookieExists` | `name` | Cookie 存在 |
| `cookieNotExists` | `name` | Cookie 不存在 |
| `cookieEquals` | `name`, `value` | Cookie 值精确匹配 |
| `cookieContains` | `name`, `value` | Cookie 值包含匹配 |
| `cookieRegex` | `name`, `pattern` | Cookie 值正则匹配 |

### Body 条件

匹配的是**请求体**（POST body），不是响应体。

| 类型 | 参数 | 说明 |
|------|------|------|
| `bodyContains` | `value` | 请求体包含指定字符串 |
| `bodyRegex` | `pattern` | 请求体正则匹配 |
| `bodyJsonPath` | `path`, `value` | JSON Path 匹配（如 `/user/role`） |

## 执行行为

### 请求阶段专属

| 类型 | 参数 | 说明 |
|------|------|------|
| `setUrl` | `value` | 修改请求 URL |
| `setMethod` | `value` | 修改 HTTP 方法 |
| `setQueryParam` | `name`, `value` | 设置查询参数 |
| `removeQueryParam` | `name` | 移除查询参数 |
| `setFormField` | `name`, `value` | 设置表单字段 |
| `removeFormField` | `name` | 移除表单字段 |
| `block` | `statusCode`, `headers`, `body`, `bodyEncoding` | 拦截并返回自定义响应（后续 action 不再执行） |

**block 示例：**

```json
{
  "type": "block",
  "statusCode": 403,
  "headers": {"Content-Type": "application/json"},
  "body": "{\"error\":\"Blocked\"}",
  "bodyEncoding": "text"
}
```

`bodyEncoding` 可选 `text` 或 `base64`。

### 响应阶段专属

| 类型 | 参数 | 说明 |
|------|------|------|
| `setStatus` | `value` (number) | 修改响应状态码 |

### 请求/响应通用

以下行为在请求和响应阶段均可使用，会根据所在阶段自动处理请求体或响应体：

| 类型 | 参数 | 说明 |
|------|------|------|
| `setHeader` | `name`, `value` | 设置请求头或响应头 |
| `removeHeader` | `name` | 移除请求头或响应头 |
| `setCookie` | `name`, `value` | 设置 Cookie（请求阶段修改 Cookie 头，响应阶段修改 Set-Cookie 头） |
| `removeCookie` | `name` | 移除 Cookie |
| `setBody` | `value`, `encoding` | 替换请求体或响应体 |
| `appendBody` | `value` | 向请求体或响应体末尾追加内容 |
| `replaceBodyText` | `search`, `replace`, `replaceAll` | 字符串替换请求体或响应体 |
| `patchBodyJson` | `patches` | JSON Patch 修改请求体或响应体（RFC 6902） |

**setBody 示例：**

```json
{"type": "setBody", "value": "{\"code\":0,\"data\":[]}", "encoding": "text"}
```

**replaceBodyText 示例：**

```json
{"type": "replaceBodyText", "search": "old", "replace": "new", "replaceAll": true}
```

**patchBodyJson 示例：**

```json
{
  "type": "patchBodyJson",
  "patches": [
    {"op": "replace", "path": "/user/name", "value": "newName"}
  ]
}
```

支持的 JSON Patch 操作：`add` / `remove` / `replace` / `move` / `copy` / `test`

## 完整配置示例

```json
{
  "id": "twist-20260611-demo01",
  "name": "演示配置",
  "version": "1.0",
  "description": "包含常见规则的演示",
  "rules": [
    {
      "id": "rule-001",
      "name": "Mock 用户信息",
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
          "value": "{\"code\":0,\"data\":{\"id\":1,\"name\":\"测试\"}}",
          "encoding": "text"
        }
      ]
    },
    {
      "id": "rule-002",
      "name": "全局 CORS",
      "enabled": true,
      "priority": 5,
      "stage": "response",
      "match": {},
      "actions": [
        {"type": "setHeader", "name": "Access-Control-Allow-Origin", "value": "*"}
      ]
    },
    {
      "id": "rule-003",
      "name": "阻止追踪",
      "enabled": true,
      "priority": 0,
      "stage": "request",
      "match": {
        "anyOf": [
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
