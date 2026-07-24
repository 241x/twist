# 命令行参数

## 基本用法

```bash
twist [参数]
```

twist 有三种运行模式：**观测**（`--observe`）、**修改**（`-c`）、**列表**（`--list-targets`），三选一。

## 参数列表

| 参数 | 短标志 | 类型 | 默认值 | 说明 |
|------|--------|------|--------|------|
| `--host` | `-H` | string | `127.0.0.1` | 浏览器 CDP 监听地址 |
| `--port` | `-p` | int | `9222` | CDP 端口。`--launch` 时作为新浏览器的远程调试端口 |
| `--launch` | — | bool | `false` | 自动启动新浏览器实例 |
| `--launch-browser` | — | string | `chrome` | 浏览器类型：`chrome` / `chromium` / `edge` |
| `--launch-args` | — | []string | — | 浏览器额外启动参数（可多次指定） |
| `--url` | `-u` | string | — | 在浏览器中打开的网址 |
| `--config` | `-c` | string | — | 规则配置文件路径（修改模式） |
| `--target` | `-t` | string | — | 附加到指定标签页 ID |
| `--list-targets` | — | bool | `false` | 列出可用标签页后退出 |
| `--observe` | — | bool | `false` | 观测模式：输出网络事件 JSONL 到 stdout |
| `--observe-count` | — | int | `0` | 观测 N 个匹配事件后退出（0 = 不限） |
| `--observe-duration` | — | string | — | 观测一段时间后退出（如 `30s`、`5m`） |
| `--observe-filter` | — | []string | — | 过滤事件（可多次指定，格式：`key=值1,值2`） |
| `--observe-full-body` | — | bool | `false` | 输出完整响应体（默认截断 4KB） |
| `--verbose` | `-v` | bool | `false` | 输出详细调试日志 |
| `--timeout` | — | int | `15` | CDP 连接超时（秒） |

## 模式互斥

三种模式互斥，同时指定会报错：

| 组合 | 结果 |
|------|------|
| `--observe` + `-c` | ❌ 互斥 |
| `--observe` + `--list-targets` | ❌ 互斥 |
| 三者均未指定 | ❌ 提示选择一种模式 |

观测子选项（`--observe-count` 等）仅在 `--observe` 时有效，单独使用会报错。

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

### 场景 2：观测网络请求

```bash
# 观测所有请求和响应
twist --launch --observe -u https://example.com

# 只看 XHR 请求，20 个事件后退出
twist --launch --observe --observe-filter type=xhr --observe-count 20

# 看 API 请求 30 秒
twist --launch --observe --observe-filter url=api --observe-duration 30s

# 看完整的响应体
twist --launch --observe --observe-full-body

# 连接已有浏览器观测
twist --observe --observe-filter url=analytics

# AI agent 消费 stdout
twist --launch --observe --observe-count 50 2>/dev/null | while read line; do
    echo "$line" | jq .
done
```

### 场景 3：启动浏览器并拦截

```bash
twist --launch -u https://www.baidu.com -c rules.json
twist --launch --launch-browser edge -p 9333 -c rules.json
twist --launch --launch-args "--headless=new" -c rules.json
```

### 场景 4：连接已有浏览器拦截

```bash
twist -c rules.json
twist -H 192.168.1.100 -p 9222 -c rules.json
twist -t ABC123 -c rules.json
twist -u https://example.com -c rules.json
```

### 场景 5：通过管道传入配置

```bash
cat rules.json | twist --launch -u https://example.com
twist --launch < rules.json
```

### 场景 6：配置文件自动查找

未指定 `-c` 时，twist 按以下优先级查找：

1. 当前目录下的 `.twist.json`
2. 当前目录下的 `twist.json`

均不存在则报错退出。

## 行为速查表

| --observe | --list-targets | --launch | --url | --target | 行为 |
|:---:|:---:|:---:|:---:|:---:|---|
| ✓ | — | ✓ | ✓ | * | 启动浏览器 → 打开 URL → 观测 |
| ✓ | — | — | ✓ | * | 连接浏览器 → 打开 URL → 观测 |
| ✓ | — | — | — | * | 连接浏览器 → 观测当前页面 |
| — | ✓ | * | * | * | 列出目标，退出 |
| — | — | ✓ | ✓ | ✓ | 启动浏览器 → 打开 URL → 附加 → 拦截 |
| — | — | ✓ | ✓ | — | 启动浏览器 → 打开 URL → 拦截 |
| — | — | ✓ | — | — | 启动浏览器 → 拦截第一个 page |
| — | — | — | ✓ | — | 连接 → 新标签页 → 拦截 |
| — | — | — | — | ✓ | 连接 → 附加 target → 拦截 |
| — | — | — | — | — | 连接 → 拦截第一个 page |

## 日志

使用 `-v` 启用详细日志，输出到 stderr：

```
2026-06-11T22:46:08+08:00 INF config loaded name=演示 rules=3
2026-06-11T22:46:08+08:00 INF browser launched browser=chrome port=9222
2026-06-11T22:46:08+08:00 INF target selected id=ABC123 url=https://example.com
2026-06-11T22:46:08+08:00 INF observation started
```

观测模式的 JSONL 事件输出到 **stdout**，日志输出到 **stderr**，互不干扰。

## 信号处理

| 信号 | 行为 |
|------|------|
| `SIGINT`（Ctrl+C） | 优雅退出：关闭 CDP 连接，若浏览器由 `--launch` 启动则关闭 |
| 再次 `SIGINT` | 强制退出，不等待清理 |
