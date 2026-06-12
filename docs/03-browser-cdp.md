# 浏览器与 CDP 交互说明

## 浏览器启动

### 自动查找

`--launch` 时，twist 按操作系统和 `--launch-browser` 类型自动查找浏览器：

| 操作系统 | chrome | edge |
|----------|--------|------|
| Windows | `%ProgramFiles%\Google\Chrome\Application\chrome.exe` 等 | `%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe` 等 |
| macOS | `/Applications/Google Chrome.app/...` | `/Applications/Microsoft Edge.app/...` |
| Linux | `google-chrome` / `google-chrome-stable` | `microsoft-edge` / `microsoft-edge-stable` |

按顺序尝试，找到第一个可用即启动；全部未找到则报错退出。

### 启动参数

twist 自动追加以下参数：

```
--remote-debugging-port={port}
--no-first-run
--no-default-browser-check
```

用户可通过 `--launch-args` 追加额外参数（如 `--headless=new`）。**不自动设置 `--user-data-dir`**，浏览器使用默认用户数据目录。

### 端口检测

启动浏览器前检测 `--port` 是否空闲。若被占用，报错提示用户更换端口或关闭占用进程。

### 生命周期

- `--launch` 启动的浏览器：twist 退出时**自动关闭**
- 连接已有浏览器：twist 退出时**不影响**浏览器

## CDP 连接

### 连接流程

```
1. HTTP GET http://{host}:{port}/json/version → 获取浏览器信息
2. 提取 webSocketDebuggerUrl
3. WebSocket 连接到 browser endpoint
4. 管理标签页（创建、列表、选择）
5. 连接到目标标签页的 WebSocket endpoint
6. 启用 Network + Fetch 域
7. 进入拦截循环
```

### 超时

`--timeout` 秒内无法连接，报错退出并提示检查浏览器是否已开启远程调试。

### 两层 WebSocket

| 层级 | 用途 | 生命周期 |
|------|------|----------|
| 浏览器级 | 管理标签页（创建、列表） | 目标选中后立即关闭 |
| 页面级 | 拦截具体页面请求 | 持续到 twist 退出 |

## 目标选择

`--target` 未指定时，twist 按以下策略选择拦截目标：

1. 若指定了 `--url`：创建新标签页 → 导航到 URL → 以此为 target
2. 若未指定 `--url`：选择**第一个 `type` 为 `page` 的标签页**

当 `--launch` + `--url` 时，URL 已作为浏览器启动参数传入，不会再创建新标签页。

## 拦截流程

### 事件循环

每个 HTTP 请求产生两次 `Fetch.RequestPaused` 事件：

```
                    ┌─────────────────────────┐
                    │   Fetch.RequestPaused    │
                    │   (ResponseStatusCode=nil)│
                    │   请求阶段                 │
                    └───────────┬───────────────┘
                                │
                    ┌───────────▼───────────────┐
                    │  匹配 stage=="request"     │
                    │  的规则 → 执行 action       │
                    │  ContinueRequest /         │
                    │  FulfillRequest            │
                    └───────────┬───────────────┘
                                │
                    ┌───────────▼───────────────┐
                    │   Fetch.RequestPaused      │
                    │   (ResponseStatusCode=200) │
                    │   响应阶段                  │
                    └───────────┬───────────────┘
                                │
                    ┌───────────▼───────────────┐
                    │  匹配 stage=="response"    │
                    │  的规则 → 执行 action       │
                    │  ContinueResponse /        │
                    │  FulfillRequest            │
                    └───────────────────────────┘
```

### 匹配原则

**所有匹配条件始终基于请求数据**（`ev.Request`）。响应头、响应状态码、响应体不参与匹配。两个阶段使用相同的数据源独立匹配。

### 前置过滤

以下请求在进入规则匹配前自动放行，不计入规则匹配结果：

1. URL scheme 非 `http://` / `https://`（data:、blob:、chrome-extension: 等）
2. WebSocket 连接
3. CORS 预检请求（OPTIONS）
4. CDP 自身请求（避免死循环）
5. `Content-Length` > 5MB（避免内存暴涨，verbose 日志可见）

### 并发处理

twist 使用单接收 + 多 Worker 架构处理高并发：

- **event loop**（单 goroutine）：接收事件 + 前置过滤。过滤放行的请求直接 `ContinueRequest`，零开销
- **worker 池**（`max(CPU核心数, 4)` 个 goroutine）：并行执行规则匹配和 action
- buffered channel（容量 100）自然反压，worker 满时浏览器请求自动暂停
- 单请求 5 秒超时，强制放行防阻塞
- panic 恢复，记录日志后继续

## 标签页输出格式

`--list-targets` 输出对齐表格：

```
ID                             TYPE  URL                       TITLE
A1B2C3D4E5F6...                page  https://example.com        Example Page
B2C3D4E5F6A1...                page  about:blank               New Tab
```
