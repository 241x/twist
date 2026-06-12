# 命令行参数

## 基本用法

```bash
twist [参数]
```

## 参数列表

| 参数 | 短标志 | 类型 | 默认值 | 说明 |
|------|--------|------|--------|------|
| `--host` | `-H` | string | `127.0.0.1` | 浏览器 CDP 监听地址 |
| `--port` | `-p` | int | `9222` | CDP 端口。`--launch` 时作为新浏览器的远程调试端口 |
| `--launch` | — | bool | `false` | 自动启动新浏览器实例 |
| `--launch-browser` | — | string | `chrome` | 浏览器类型：`chrome` / `chromium` / `edge` |
| `--launch-args` | — | []string | — | 浏览器额外启动参数（可多次指定） |
| `--url` | `-u` | string | — | 在浏览器中打开的网址 |
| `--config` | `-c` | string | — | 规则配置文件路径 |
| `--target` | `-t` | string | — | 附加到指定标签页 ID |
| `--list-targets` | — | bool | `false` | 列出可用标签页后退出 |
| `--verbose` | `-v` | bool | `false` | 输出详细调试日志 |
| `--timeout` | — | int | `15` | CDP 连接超时（秒） |

## 退出码

| 退出码 | 含义 |
|--------|------|
| `0` | 正常退出 |
| `1` | 用户侧错误（参数错误、配置缺失、浏览器未运行等） |
| `2` | 运行时错误（连接中断、CDP 协议错误等） |

## 使用场景

### 场景 1：列出浏览器标签页

```bash
twist --list-targets
twist --list-targets -p 9333
```

连接已有浏览器，列出所有标签页 ID、URL 和标题，然后退出。不加载规则。

### 场景 2：启动浏览器并拦截

```bash
# 启动 Chrome，打开百度，加载规则
twist --launch -u https://www.baidu.com -c rules.json

# 启动 Edge，指定调试端口
twist --launch --launch-browser edge -p 9333 -c rules.json

# 启动无头模式
twist --launch --launch-args "--headless=new" -c rules.json

# 启动后附加到指定标签页
twist --launch -u https://example.com -t ABC123 -c rules.json
```

### 场景 3：连接已有浏览器

```bash
# 连接本地默认端口
twist -c rules.json

# 连接远程浏览器
twist -H 192.168.1.100 -p 9222 -c rules.json

# 连接到指定标签页
twist -t ABC123 -c rules.json

# 在新标签页中打开网址
twist -u https://example.com -c rules.json
```

### 场景 4：通过管道传入配置

```bash
cat rules.json | twist --launch -u https://example.com
twist --launch < rules.json
```

当同时指定 `--config` 和管道输入时，优先使用 `--config` 指定的文件。

### 场景 5：配置文件自动查找

未指定 `--config` 时，twist 按以下优先级查找默认配置：

1. 当前目录下的 `.twist.json`
2. 当前目录下的 `twist.json`

均不存在则报错退出。

## 行为速查表

| --list-targets | --launch | --url | --target | 行为 |
|:---:|:---:|:---:|:---:|---|
| ✓ | * | * | * | 列出目标，退出 |
| — | ✓ | ✓ | ✓ | 启动浏览器 → 打开 URL → 附加 target → 拦截 |
| — | ✓ | ✓ | — | 启动浏览器 → 打开 URL → 拦截该页面 |
| — | ✓ | — | ✓ | 启动浏览器（空白页）→ 附加 target → 拦截 |
| — | ✓ | — | — | 启动浏览器（空白页）→ 拦截第一个 page |
| — | — | ✓ | ✓ | 连接浏览器 → 导航 target 到 URL → 拦截 |
| — | — | ✓ | — | 连接浏览器 → 新标签页打开 URL → 拦截 |
| — | — | — | ✓ | 连接浏览器 → 附加 target → 拦截 |
| — | — | — | — | 连接浏览器 → 拦截第一个 page |

## 日志

使用 `-v` 启用详细日志，输出到 stderr：

```
2026-06-11T22:46:08+08:00 INF config loaded name=演示 rules=3
2026-06-11T22:46:08+08:00 INF browser launched browser=chrome port=9222
2026-06-11T22:46:08+08:00 INF CDP connected host=127.0.0.1 port=9222
2026-06-11T22:46:08+08:00 INF target selected id=ABC123 url=https://example.com
2026-06-11T22:46:08+08:00 INF interception started
```

Bypass（放行）的请求按 `DEBUG` 级别记录，仅 `-v` 时可见。

## 信号处理

| 信号 | 行为 |
|------|------|
| `SIGINT`（Ctrl+C） | 优雅退出：关闭 CDP 连接，若浏览器由 `--launch` 启动则关闭 |
| 再次 `SIGINT` | 强制退出，不等待清理 |
