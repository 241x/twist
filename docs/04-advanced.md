# 进阶说明

## 架构设计

```
cmd/twist/main.go          → 入口，退出码处理
internal/cmd/               → Cobra 命令、参数解析、配置加载
internal/app/               → 业务逻辑层
  app.go                    → 编排器，串联完整流程
  browser.go                → 跨平台浏览器启动与管理
  cdp.go                    → CDP 双 WebSocket 连接管理
  target.go                 → 标签页选择策略
  config.go                 → JSON 规则解析与字段校验
  intercept.go              → Fetch 域事件循环、规则匹配、action 执行
internal/log/               → zerolog 结构化日志
```

## 规则执行优先级

同一阶段内，规则按 `priority` 降序执行。同一规则内，actions 按数组顺序执行。

```
请求到达 → 按 priority 降序匹配 stage=="request" 规则 → 执行匹配的 actions
  → 发送请求 → 响应到达 → 按 priority 降序匹配 stage=="response" 规则 → 执行匹配的 actions
  → 返回浏览器
```

`block` 是终结性行为，执行后不再执行后续 action。

## 匹配数据源

所有匹配条件的数据源均为 `ev.Request`（请求数据），不受 `stage` 影响：

| 条件组 | 数据源 |
|--------|--------|
| URL 条件（5种） | `ev.Request.URL` |
| method | `ev.Request.Method` |
| resourceType | `ev.ResourceType` |
| header 条件（5种） | `ev.Request.Headers`（请求头） |
| query 条件（5种） | 从 `ev.Request.URL` 解析 |
| cookie 条件（5种） | 从 `ev.Request.Headers["Cookie"]` 解析 |
| body 条件（3种） | `ev.Request.PostData`（请求体） |

## 无需拦截的请求

以下类型的请求会被自动放行，避免阻塞浏览器：

| 类型 | 判定 |
|------|------|
| 非 HTTP scheme | `data:` / `blob:` / `chrome-extension:` 开头 |
| WebSocket | `resourceType == "WebSocket"` |
| CORS 预检 | `method == "OPTIONS"` |
| CDP 自身 | Host 匹配 `--host`:`--port` |
| 大请求体 | `Content-Length > 5MB` |

## 响应体获取

`setBody`、`replaceBodyText`、`patchBodyJson` 在响应阶段执行，需要先通过 `Fetch.GetResponseBody` 获取原始响应体。

CDP 可能返回 base64 编码的响应体（`Reply.Base64Encoded == true`），twist 会自动解码后处理。

`GetResponseBody` 是一次额外的 RPC 往返，带来约 10-50ms 延迟。

## 正则表达式

- 语法：Go 标准 `regexp` 包（RE2），不支持反向引用和前瞻
- 缓存：首次编译后存入 `sync.Map`，后续复用，避免重复编译
- 异常：模式编译失败时返回 `false`（不匹配），不 panic

## 性能考虑

| 操作 | 耗时 |
|------|------|
| 前置过滤放行 | < 1ms（一次 ContinueRequest RPC） |
| 规则匹配（无命中） | < 1ms（纯内存） |
| 规则匹配 + action | 1-10ms（CPU + RPC） |
| 响应体操作（GetResponseBody） | +10-50ms |

## 退出码

| 退出码 | 含义 |
|--------|------|
| `0` | 正常退出 |
| `1` | 用户侧错误（参数错误、配置缺失、浏览器未运行等） |
| `2` | 运行时错误（连接中断、CDP 协议错误等） |

## 依赖库

| 库 | 用途 |
|----|------|
| `github.com/spf13/cobra` | CLI 命令框架 |
| `github.com/mafredri/cdp` | CDP 协议客户端 |
| `github.com/rs/zerolog` | 结构化日志 |

## 环境要求

- Go 1.26+
- Chrome / Chromium / Edge（自动启动时）
- 或任意支持 `--remote-debugging-port` 的浏览器
