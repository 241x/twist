# CDP 连接与拦截引擎设计文档

## 依赖

使用 `github.com/mafredri/cdp` 及其子包：

| 包 | 用途 |
|----|------|
| `cdp/devtool` | HTTP `/json` 端点：列出目标、创建标签、获取 WebSocket URL |
| `cdp/rpcc` | WebSocket 连接 |
| `cdp` | 聚合客户端，封装所有 CDP 域操作 |
| `cdp/protocol/target` | Target 域：管理标签页 |
| `cdp/protocol/page` | Page 域：导航 |
| `cdp/protocol/network` | Network 域：启用网络监听 |
| `cdp/protocol/fetch` | Fetch 域：请求/响应拦截（推荐，替代已弃用的 Network.setRequestInterception） |

---

## 整体架构

```
┌─────────────────────────────────────────────┐
│                    App                       │
│  runListTargets    │    runIntercept         │
└───────┬────────────┴──────────┬──────────────┘
        │                       │
        ▼                       ▼
┌───────────────┐     ┌─────────────────────────┐
│     CDP       │     │         CDP             │
│  (browser)    │     │   (browser + target)    │
└───────┬───────┘     └────┬───────────┬────────┘
        │                  │           │
        ▼                  ▼           ▼
  devtool.DevTools    Target选择   Intercept引擎
  (HTTP /json)        (target.go)  (intercept.go)
```

---

## CDP 结构体重设计

```go
type CDP struct {
    host    string
    port    int
    timeout int
    verbose bool

    devt   *devtool.DevTools      // HTTP 端点客户端（浏览器级）
    conn   *rpcc.Conn             // 浏览器级 WebSocket 连接
    client *cdp.Client            // 浏览器级 CDP 客户端（Target 域操作）
    
    targetConn   *rpcc.Conn       // 目标页面级 WebSocket 连接
    targetClient *cdp.Client      // 目标页面级 CDP 客户端（Page/Network/Fetch 域操作）
}
```

### 两种连接模式

| 连接 | 用途 | 获取方式 |
|------|------|----------|
| 浏览器级 | 管理标签页（创建、附加） | `devtool.Version()` → `rpcc.Dial(wsURL)` |
| 页面级 | 拦截具体页面的请求/响应 | `devtool.Target.WebSocketDebuggerURL` → `rpcc.Dial(wsURL)` |

---

## 连接流程

### 1. 列出目标（`--list-targets`）

```
1. devtool.New("http://{host}:{port}")
2. devt.List(ctx) → []devtool.Target
3. 格式化输出 → 退出
```

无需 WebSocket 连接，仅 HTTP 请求。

### 2. 拦截模式（正常运行时）

```
1. devtool.New("http://{host}:{port}")
2. 等待浏览器就绪（轮询 devt.Version()，最多 --timeout 秒）
3. devt.Version() → 获取 browser WebSocket URL
4. rpcc.DialContext(browserWSURL) → browser conn
5. cdp.NewClient(browserConn) → browser client
6. 选择目标（见下方）
7. 获取目标页面的 WebSocket URL
8. rpcc.DialContext(targetWSURL) → target conn  
9. cdp.NewClient(targetConn) → target client
10. targetClient.Network.Enable()
11. targetClient.Fetch.Enable(patterns)
12. 进入拦截循环，监听 Fetch.RequestPaused 事件
```

---

## 目标选择逻辑（`Target.Select`）

```
输入: ctx, targetID, url

if targetID != "":
    1. devt.List(ctx) → 遍历查找 target.ID == targetID
    2. 找到 → 返回该 target；未找到 → error

elif url != "":
    1. devt.CreateURL(ctx, url) → 新标签打开 URL
    2. 返回新标签的 target

else:
    1. devt.List(ctx) → 过滤 Type == "page"
    2. 返回第一个 page 类型 target；无则 error
```

特殊：`--launch` + `--url` 时 URL 已作为浏览器启动参数传入，页面已打开。此时 `Target.Select` 的 URL 参数应传空字符串，避免重复创建标签。此判断在 `App.runIntercept` 中完成：

```go
// App.runIntercept 中
selectURL := a.opts.URL
if a.opts.Launch {
    selectURL = ""  // URL 已在浏览器启动时传入
}
selected, err := a.target.Select(ctx, a.opts.Target, selectURL)
```

---

## 浏览器级连接生命周期

浏览器级连接仅用于 Target 域操作（创建标签）。目标选中后不再需要，应立即关闭释放资源：

```go
// 目标页面 WS 连接建立后
a.cdp.CloseBrowser()  // 关闭浏览器级连接，仅保留页面级连接
```

`--list-targets` 模式始终使用 HTTP（devtool），不建立 WebSocket 连接。

---

## 断线处理

| 连接类型 | 断开场景 | 行为 |
|----------|----------|------|
| 浏览器级 WS | 建立后、目标选中前断开 | 报错退出 |
| 页面级 WS | 拦截循环中 `paused.Recv()` 返回错误 | 退出拦截，清理退出 |
| HTTP (devtool) | `devt.Version()` / `devt.List()` 失败 | 重试 `--timeout` 秒，超时则报错退出 |

一期不实现自动重连，断开后退出由用户重新执行命令。

---

## 拦截引擎（`Intercept`）

### 数据流

```
浏览器 → Fetch.RequestPaused 事件 → 匹配条件 → 执行 Action → 返回浏览器
```

### 事件处理循环

```go
func (i *Intercept) loop(ctx context.Context) error {
    paused, _ := i.targetClient.Fetch.RequestPaused(ctx)
    defer paused.Close()

    workerCh := make(chan *fetch.RequestPausedReply, 100)

    // 启动 worker 池
    var wg sync.WaitGroup
    for j := 0; j < i.workerCount; j++ {
        wg.Add(1)
        go i.worker(ctx, workerCh, &wg)
    }

    // 事件接收循环
    for {
        select {
        case <-ctx.Done():
            close(workerCh)
            wg.Wait()
            return ctx.Err()
        default:
        }

        ev, err := paused.Recv()
        if err != nil {
            close(workerCh)
            wg.Wait()
            return err
        }

        // 前置过滤：快速放行
        if i.shouldBypass(ev) {
            i.targetClient.Fetch.ContinueRequest(ctx,
                fetch.NewContinueRequestArgs(ev.RequestID))
            continue
        }

        // 需要规则匹配，交给 worker
        select {
        case workerCh <- ev:
        case <-ctx.Done():
            close(workerCh)
            wg.Wait()
            return ctx.Err()
        }
    }
}

func (i *Intercept) worker(ctx context.Context, ch <-chan *fetch.RequestPausedReply, wg *sync.WaitGroup) {
    defer wg.Done()
    for ev := range ch {
        i.processEvent(ctx, ev)
    }
}

func (i *Intercept) processEvent(ctx context.Context, ev *fetch.RequestPausedReply) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    rule := i.matchRules(ev, "request")
    if rule == nil {
        i.targetClient.Fetch.ContinueRequest(ctx,
            fetch.NewContinueRequestArgs(ev.RequestID))
        return
    }
    i.executeActions(ctx, ev, rule)
}
```

### 规则匹配（`matchRules`）

```go
func (i *Intercept) matchRules(ev *fetch.RequestPausedReply, stage string) *Rule {
    // 1. 过滤 stage 匹配且 enabled 的规则
    // 2. 按 priority 降序排列
    // 3. 对每条规则：
    //    a. allOf: 所有条件必须满足
    //    b. anyOf: 至少一个条件满足（empty = 匹配）
    //    c. 均满足 → 返回该规则
    // 4. 无匹配 → nil
}
```

匹配条件需从事件中提取信息：
- URL → `ev.Request.URL`
- Method → `ev.Request.Method`
- Headers → `ev.Request.Headers`（`map[string]interface{}`）
- ResourceType → `ev.ResourceType`

注意：请求阶段只能访问请求数据；响应阶段需要额外等待 `Network.responseReceived` 事件获取响应头和状态码，等待 `Network.loadingFinished` 获取响应体。**一期先实现请求阶段拦截，响应阶段后续补齐。**

### Action 执行

| Action | 对应 CDP 方法 | 说明 |
|--------|--------------|------|
| `setUrl` | 修改 ev.Request.URL → ContinueRequest | URL 修改需要重新构造请求 |
| `setMethod` | 同上 | |
| `setHeader` / `removeHeader` | Fetch.ContinueRequest 设置 headers | |
| `setQueryParam` / `removeQueryParam` | 修改 URL → ContinueRequest | |
| `setCookie` / `removeCookie` | Network 域的 Cookie 方法 | 二期 |
| `block` | Fetch.FulfillRequest / Fetch.FailRequest | 终结性 |
| 其余通用 action | ContinueRequest / 不调用（放行） | |

**一期简化**：请求阶段仅实现 `block`（FulfillRequest/FailRequest）和 `ContinueRequest`（放行/修改 headers）。其他 action 后续逐一实现。

---

## 无需拦截的请求类型

以下类型的网络请求不适合拦截处理，应在 Fetch 事件循环中**立即放行（ContinueRequest）**，避免阻塞浏览器：

### 1. WebSocket

- **特征**：`resourceType == "websocket"` 或请求头包含 `Upgrade: websocket`
- **原因**：WebSocket 是长连接双向通信协议，Fetch 域的暂停/恢复机制会破坏握手流程。拦截 WebSocket 升级请求将导致连接建立失败。
- **处理**：检测到后立即 `ContinueRequest`，不匹配规则。

### 2. EventSource / SSE

- **特征**：`resourceType` 可能标记为 `fetch` 或 `xhr`，响应头 `Content-Type: text/event-stream`
- **原因**：服务端推送事件是持久流式连接，暂停后恢复可能丢失数据或导致连接中断。
- **处理**：请求阶段无可靠标识，暂在 Fetch 启用时不拦截 `*` 全量模式，改为按需设置 pattern；或允许用户通过规则显式排除。

### 3. 大请求体 / 大响应体

- **特征**：请求头 `Content-Length` > 阈值（如 5MB），或 `Transfer-Encoding: chunked`
- **原因**：`Fetch.RequestPaused` 暂存完整请求体，大体积会导致内存暴涨和暂停时间过长，影响浏览器响应。
- **处理**：请求阶段检测 `Content-Length` 值，超过阈值直接放行。响应阶段同样处理。

### 4. 预检请求（OPTIONS）

- **特征**：`method == "OPTIONS"` 且带有 `Access-Control-Request-Method` 头
- **原因**：CORS 预检请求由浏览器自动管理，拦截后可能导致跨域请求失败。
- **处理**：默认放行 OPTIONS 预检请求。用户可通过规则的 `method` 条件显式匹配来覆盖此行为。

### 5. data: / blob: / chrome-extension: 等非 HTTP URL

- **特征**：URL 以 `data:`、`blob:`、`chrome-extension:`、`devtools:`、`about:` 等开头
- **原因**：这些不是实际网络请求，拦截无意义且可能干扰浏览器内部逻辑。
- **处理**：检测 URL scheme，非 `http://` / `https://` 的请求直接放行。

### 6. 内部 DevTools 请求

- **特征**：URL 包含 `/json`、`devtools://` 或 Host 为 CDP 自身地址（`127.0.0.1:{port}`）
- **原因**：拦截 twist 自身的 CDP 通信将导致死循环或连接断开。
- **处理**：请求 Host 匹配 CDP 地址时直接放行。

### 7. Service Worker / Cache Storage

- **特征**：`resourceType` 中的特定类型，或来自 `chrome-extension://`、Service Worker scope
- **原因**：这些请求由 Service Worker 管理，拦截可能破坏缓存策略或导致 SW 异常。
- **处理**：默认不拦截。后续可通过 `Fetch.Enable` 的 `patterns` 参数精细控制。

---

## 拦截安全边界

在事件循环入口处建立**前置过滤链**，按以下顺序快速放行：

```
RequestPaused 事件到达
  │
  ├─ 1. URL scheme 非 http/https  → ContinueRequest（放行）
  ├─ 2. resourceType == websocket → ContinueRequest
  ├─ 3. method == OPTIONS（CORS）  → ContinueRequest
  ├─ 4. Host 为 CDP 自身地址     → ContinueRequest
  ├─ 5. Content-Length > 5MB      → ContinueRequest
  │
  └─ 6. 进入规则匹配引擎
```

> 步骤 1-5 的放行**不计入**规则匹配结果，`--verbose` 模式下输出 debug 日志记录放行原因。

### Fetch.Enable 的 patterns 配置策略

```go
fetch.NewEnableArgs().SetPatterns([]fetch.RequestPattern{
    {URLPattern: stringPtr("http://*/*"),  RequestStage: strPtr("Request")},
    {URLPattern: stringPtr("https://*/*"), RequestStage: strPtr("Request")},
})
```

仅对 `http://` 和 `https://` 启用拦截，其余 scheme 由 CDP 自动忽略，减少无效事件。

### Content-Length 阈值

默认 `5MB`，后续可通过 `--max-body-size` 参数或配置文件 `settings.maxBodySize` 调整。

---

## 并发处理模型

### 问题分析

打开一个典型页面时，浏览器可能同时发起 100-300 个资源请求（JS、CSS、图片、XHR、字体等）。Fetch 域会逐一暂停这些请求并发送 `RequestPaused` 事件。**每个事件只有在我们调用了 ContinueRequest / FulfillRequest / FailRequest 之后才会恢复**。如果串行处理，即使单次匹配仅耗时 1ms，第 200 个请求也要等待 199ms——页面加载将明显变慢。

### 方案：单接收 + 多 Worker + 前置快速放行

```
                    ┌─────────────────────────────────┐
                    │      Fetch.RequestPaused 事件流    │
                    └──────────────┬──────────────────┘
                                   │
                          ┌────────▼────────┐
                          │   event loop     │  ← 单一 goroutine 接收
                          │  recv + 前置过滤   │
                          │  (scheme/ws等直接  │
                          │   ContinueRequest) │
                          └────────┬────────┘
                                   │  需要规则匹配的事件
                    ┌──────────────┼──────────────┐
                    │              │              │
               ┌────▼────┐   ┌────▼────┐   ┌────▼────┐
               │ worker 1 │   │ worker 2 │   │ worker N │
               │ match    │   │ match    │   │ match    │
               │ execute  │   │ execute  │   │ execute  │
               └──────────┘   └──────────┘   └──────────┘
```

- **event loop** — 单 goroutine 从 `RequestPaused` 接收事件。前置过滤放行的请求（图片/CSS/JS 等多数无匹配规则的请求）**直接调用 ContinueRequest**，不经过 channel，零额外开销。
- **worker 池** — N 个 goroutine（`max(runtime.NumCPU(), 4)`），通过 buffered channel 接收事件，独立执行规则匹配和 action。`cdp.Client` 底层 `rpcc.Conn` 写操作由 mutex 保护，多 goroutine 调用安全。

### 事件积压保护

当 worker 池全部忙碌时，event loop 阻塞在 channel 发送，自然反压 CDP 事件流——浏览器请求保持暂停直到有 worker 空闲。这是期望行为，相较于无界队列更安全。

### 超时保护

单个请求处理超时（默认 5 秒）后**强制 ContinueRequest 放行**，防止某个复杂规则导致永久阻塞。超时请求计入 `--verbose` 日志告警。

### Worker panic 恢复

规则匹配涉及正则表达式和 JSON Path 操作，可能因非法输入导致 panic。每个 worker 内部包裹 `recover`，panic 时记录日志并 ContinueRequest 放行，不终止进程：

```go
func (i *Intercept) worker(ctx context.Context, ch <-chan *fetch.RequestPausedReply, wg *sync.WaitGroup) {
    defer wg.Done()
    defer func() {
        if r := recover(); r != nil {
            log.FromContext(ctx).Error().Interface("panic", r).Msg("worker panic recovered")
        }
    }()
    for ev := range ch {
        i.processEvent(ctx, ev)
    }
}
```

---

## 错误处理

| 场景 | 行为 |
|------|------|
| 连接超时 | `--timeout` 秒内 devt.Version() 失败 → 报错退出 |
| WebSocket 断开 | 拦截循环中检测到错误 → 退出拦截，清理连接 |
| 目标不存在 | 选择 target 时 ID 未找到 → 报错退出 |
| 目标已关闭 | Fetch.RequestPaused 收到错误 → 退出拦截 |
| 规则匹配异常 | 跳过该规则，继续匹配下一条 |

---

## 优雅关闭

```
收到 SIGINT/Ctrl+C:
1. cancel ctx → 拦截循环中的 select 捕获 ctx.Done()
2. close(workerCh) → 等待所有 worker 完成（wg.Wait()）
3. targetClient/conn.Close()      ← 先关闭页面级连接
4. browserClient/conn.Close()     ← 再关闭浏览器级连接
5. 若 browser 由 --launch 启动 → browser.Stop()
```

关闭顺序：先页面后浏览器，先 CDP 连接后进程。

---

## 一期实现范围

| 功能 | 状态 |
|------|------|
| `cdp.Connect` — 连接浏览器，建立 browser client | ✅ 一期 |
| `cdp.Close` — 关闭所有连接 | ✅ 一期 |
| `cdp.ListTargets` — 列出所有目标 | ✅ 一期 |
| `cdp.NewTab` — 创建新标签 | ✅ 一期 |
| `cdp.NavigateTo` — 导航到 URL | ✅ 一期 |
| `cdp.AttachToTarget` — 连接到目标页面 WebSocket | ✅ 一期 |
| `cdp.EnableNetwork` — 启用 Network 域 | ✅ 一期 |
| `cdp.EnableFetch` — 启用 Fetch 拦截 | ✅ 一期 |
| `Target.Select` — 目标选择策略 | ✅ 一期 |
| `Intercept.Start` — 启动拦截循环（请求阶段） | ✅ 一期 |
| 规则匹配引擎 | ✅ 一期 |
| `block` action | ✅ 一期 |
| `ContinueRequest`（放行/修改 headers） | ✅ 一期 |
| 响应阶段拦截 | ⏳ 二期 |
| `setBody` / `replaceBodyText` / `patchBodyJson` | ⏳ 二期 |
| `setUrl` / `setMethod` 等请求修改 | ⏳ 二期 |
| 条件中的 body 匹配（`bodyContains` 等） | ⏳ 二期 |
| `printTargets` 格式化输出 | ✅ 一期 |

---

## 文件变更

| 文件 | 变更 |
|------|------|
| `internal/app/cdp.go` | 重写，实现全部 CDP 操作 |
| `internal/app/target.go` | 重写，使用 devtool 实现目标选择 |
| `internal/app/intercept.go` | 重写，实现事件循环 + 规则匹配 + action 执行 |
| `internal/app/app.go` | 微调，移除不再需要的空调用 |
| `go.mod` | 新增 `github.com/mafredri/cdp` 依赖 |
