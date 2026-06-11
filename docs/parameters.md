# Twist 参数组合约束文档

## 参数总览

| 参数 | 短标志 | 类型 | 默认值 | 说明 |
|------|--------|------|--------|------|
| `--host` | `-H` | string | `127.0.0.1` | CDP 监听地址 |
| `--port` | `-p` | int | `9222` | 连接已有浏览器时作为 CDP 端口；`--launch` 时作为新浏览器的远程调试端口 |
| `--launch` | — | bool | `false` | 启动新浏览器实例 |
| `--launch-browser` | — | string | `chrome` | 浏览器类型（chrome / chromium / edge） |
| `--launch-args` | — | []string | — | 浏览器额外启动参数 |
| `--url` | `-u` | string | — | 要在浏览器中打开的网址 |
| `--config` | `-c` | string | — | 规则配置文件路径 |
| `--list-targets` | — | bool | `false` | 列出浏览器选项卡目标 |
| `--target` | `-t` | string | — | 附加到指定选项卡 |
| `--verbose` | `-v` | bool | `false` | 输出详细日志 |
| `--timeout` | — | int | `30` | CDP 连接超时（秒） |

---

## 全局约束

1. `--verbose` 和 `--timeout` 为**通用参数**，在所有场景下均生效，不参与行为互斥。
2. `--config` 为**通用参数**，在任何运行态场景下均可携带；仅在 `--list-targets` 时忽略。

---

## 默认行为

### CDP 连接流程

1. 通过 HTTP 请求 `http://{host}:{port}/json` 获取浏览器可调试端点列表。
2. 从返回的 JSON 中提取 WebSocket URL，建立 WebSocket 连接。
3. 若 `--timeout` 秒内无法建立连接，报错退出。

### 浏览器路径自动查找（`--launch` 时）

若未通过 `--launch-args` 指定完整路径，按 `--launch-browser` 类型在常见安装位置依次查找：

| 操作系统 | chrome | chromium | edge |
|----------|--------|----------|------|
| Windows | `%ProgramFiles%\Google\Chrome\Application\chrome.exe` / `%LocalAppData%\Google\Chrome\Application\chrome.exe` | `%LocalAppData%\Chromium\Application\chrome.exe` | `%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe` / `%ProgramFiles%\Microsoft\Edge\Application\msedge.exe` |
| macOS | `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` | `/Applications/Chromium.app/Contents/MacOS/Chromium` | `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge` |
| Linux | `google-chrome` / `google-chrome-stable` | `chromium` / `chromium-browser` | `microsoft-edge` / `microsoft-edge-stable` |

按表格顺序依次尝试，找到第一个可执行文件即使用；全部未找到则报错退出。

### 浏览器默认启动参数（`--launch` 时）

除用户通过 `--launch-args` 指定的参数外，自动追加以下默认参数：

```
--remote-debugging-port={port}
--no-first-run
--no-default-browser-check
```

不自动指定 `--user-data-dir`，浏览器使用其默认的用户数据目录。

### 规则配置（`--config` 未指定时）

未指定 `--config` 时，按以下优先级查找默认配置文件：

1. 当前目录下的 `.twist.json`
2. 当前目录下的 `twist.json`

若均不存在，**报错退出**，提示用户需要指定规则配置文件。

### 通过标准输入加载配置

支持通过管道将配置内容传入：

```
cat rules.json | twist
twist < rules.json
```

管道传入的配置与 `--config` 指定的文件**格式相同**。规则如下：

1. 若同时指定了 `--config` 和管道输入，优先使用 `--config` 指定的文件，忽略管道输入。
2. 若仅通过管道传入，则读取 stdin 作为配置内容。
3. 若 `--list-targets` 被指定，忽略管道输入。
4. stdin 无数据且无 `--config` 且无默认配置文件 → 报错退出。

### 浏览器未运行（无 `--launch` 时）

若未指定 `--launch` 且 `--host`:`--port` 无法连接：

1. 以 `--verbose` 模式输出连接失败详情。
2. 提示用户先启动浏览器并开启远程调试，或使用 `--launch` 自动启动。
3. 报错退出。

### 选项卡选择（无 `--target` 时）

连接浏览器后，若无 `--target` 指定，选择策略：

1. 若指定了 `--url`：创建新选项卡 → 导航到 URL → 以此为 target。
2. 若未指定 `--url`：选择**第一个 `type` 为 `page` 的目标**；若无任何 page 类型目标则报错退出。

### 浏览器生命周期

若浏览器由 `--launch` 启动（即 twist 创建的进程），twist 退出时**自动关闭**该浏览器。

若连接到已有浏览器（未指定 `--launch`），twist 退出时**不影响**浏览器运行。

### 端口占用检测（`--launch` 时）

启动浏览器前，先检测 `--port` 是否已被占用：

- 端口空闲 → 正常启动。
- 端口被占用 → **报错退出**，提示用户更换 `--port` 或关闭占用端口的进程。

### 信号处理

| 信号 | 行为 |
|------|------|
| `SIGINT`（Ctrl+C） | 优雅退出：关闭 WebSocket 连接，若浏览器由 `--launch` 启动则关闭浏览器，正常退出 |
| `SIGTERM` | 同 `SIGINT` |

收到两次 `SIGINT`/`SIGTERM` 则**强制退出**，不等待清理。

### 退出码

| 退出码 | 含义 |
|--------|------|
| `0` | 正常退出 |
| `1` | 参数错误、配置缺失、浏览器未运行等用户侧错误 |
| `2` | 运行时报错（连接中断、CDP 协议错误、规则执行失败等） |

### `--list-targets` 输出格式

```
ID                                     URL                          Title
A1B2C3D4E5F6...                       https://example.com          Example Page
B2C3D4E5F6A1...                       about:blank                  New Tab
```



---

## 场景一：`--list-targets`

**触发条件**：指定了 `--list-targets`。

**行为**：
1. 连接到 `--host`:`--port` 指定的浏览器 CDP 端点。
2. 输出所有选项卡目标（ID、URL、标题）。
3. **立即退出**，不加载规则、不拦截请求。
4. 其他参数（`--launch`、`--url`、`--target`、`--config`）**均被忽略**。

**错误**：
- 连接超时 → 报错退出。

---

## 场景二：`--launch`（启动新浏览器）

### 2.1 `--launch` + `--url` + `--target`

1. 以 `--port` 为远程调试端口启动新浏览器实例（`--launch-browser` 指定类型，`--launch-args` 追加参数）。
2. 浏览器启动参数中携带 `--url` 指定的网址。
3. 连接到 `127.0.0.1`:`--port` 的 CDP 端点。
4. 通过 `--target` 指定的选项卡 ID 进行附加。
5. 加载 `--config` 规则，开始拦截。

**错误**：
- 浏览器启动失败 → 报错退出。
- 指定 `--target` 不存在 → 报错退出。

### 2.2 `--launch` + `--url`（无 `--target`）

1. 以 `--port` 为远程调试端口启动新浏览器实例，启动参数中携带 `--url`。
2. 连接到 `127.0.0.1`:`--port` 的 CDP 端点。
3. 以刚打开的页面为目标，加载 `--config` 规则，开始拦截。

### 2.3 `--launch`（无 `--url`、无 `--target`）

1. 以 `--port` 为远程调试端口启动新浏览器实例（默认空白页）。
2. 连接到 `127.0.0.1`:`--port` 的 CDP 端点。
3. 以第一个 page 类型选项卡为目标，加载 `--config` 规则，开始拦截。

### 2.4 `--launch` + `--target`（无 `--url`）

1. 以 `--port` 为远程调试端口启动新浏览器实例（默认空白页）。
2. 连接到 `127.0.0.1`:`--port` 的 CDP 端点。
3. 通过 `--target` 附加到指定选项卡。
4. 加载 `--config` 规则，开始拦截。

---

## 场景三：连接已有浏览器（无 `--launch`）

### 3.1 `--url` + `--target`

1. 连接到 `--host`:`--port` 的已有浏览器。
2. 在 `--target` 指定的选项卡中导航到 `--url`。
3. 加载 `--config` 规则，开始拦截。

**错误**：
- `--target` 不存在 → 报错退出。

### 3.2 `--url`（无 `--target`）

1. 连接到 `--host`:`--port` 的已有浏览器。
2. **创建新选项卡**并导航到 `--url`。
3. 加载 `--config` 规则，开始拦截。

### 3.3 `--target`（无 `--url`）

1. 连接到 `--host`:`--port` 的已有浏览器。
2. 附加到 `--target` 指定的选项卡。
3. **不导航**，加载 `--config` 规则，开始拦截。

**错误**：
- `--target` 不存在 → 报错退出。

### 3.4 无 `--url`、无 `--target`

1. 连接到 `--host`:`--port` 的已有浏览器。
2. 以**第一个 `type` 为 `page` 的目标**作为拦截目标。
3. 加载 `--config` 规则，开始拦截。

**错误**：
- 无 page 类型目标 → 报错退出。

---

## 参数互斥与冲突一览

| 参数A | 参数B | 关系 |
|-------|-------|------|
| `--list-targets` | `--launch` | `--list-targets` 优先，`--launch` 忽略 |
| `--list-targets` | `--url` | `--list-targets` 优先，`--url` 忽略 |
| `--list-targets` | `--target` | `--list-targets` 优先，`--target` 忽略 |
| `--list-targets` | `--config` | `--list-targets` 优先，`--config` 忽略 |
| `--launch` | `--host` | `--launch` 时忽略 `--host`，始终使用本地地址 |
| `--launch` | `--port` | `--launch` 时 `--port` 作为新浏览器的远程调试端口 |
| `--launch` | `--target` | 兼容，见 2.1 / 2.4 |
| `--url` | `--target` | 兼容，见 2.1 / 3.1 |

---

## 行为速查表

| --list-targets | --launch | --url | --target | 行为摘要 |
|:---:|:---:|:---:|:---:|---|
| ✓ | * | * | * | 列出目标，退出 |
| — | ✓ | ✓ | ✓ | 启动浏览器 + 打开 URL → 附加指定 target → 拦截 |
| — | ✓ | ✓ | — | 启动浏览器 + 打开 URL → 拦截该页面 |
| — | ✓ | — | ✓ | 启动浏览器（空白页）→ 附加指定 target → 拦截 |
| — | ✓ | — | — | 启动浏览器（空白页）→ 拦截第一个 page 目标 |
| — | — | ✓ | ✓ | 连接已有浏览器 → 导航 target 到 URL → 拦截 |
| — | — | ✓ | — | 连接已有浏览器 → 新选项卡打开 URL → 拦截 |
| — | — | — | ✓ | 连接已有浏览器 → 附加 target → 拦截 |
| — | — | — | — | 连接已有浏览器 → 拦截第一个 page 目标 |

> `*` = 任意值，`✓` = 指定，`—` = 未指定
