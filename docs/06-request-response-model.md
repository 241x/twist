# 请求/响应两阶段处理模型

## 核心原则

1. **匹配条件只工作在请求数据上**——所有条件（URL/method/header/query/cookie/body）均基于 `ev.Request` 进行匹配。响应头、响应状态码、响应体**不作为匹配条件的数据源**。
2. **stage 字段同时约束匹配过滤和 action 时机**——`stage:"request"` 的规则在请求事件到达时匹配并执行；`stage:"response"` 的规则在响应事件到达时匹配并执行。
3. **两次事件独立匹配，无需缓存**——`Fetch.RequestPaused` 的响应阶段事件中 `ev.Request` 数据与请求阶段完全一致，可直接重新匹配。

---

## 事件流

```
浏览器发起 GET /api/users
    │
    ├─▶ Fetch.RequestPaused (RequestID=A, ResponseStatusCode=nil)
    │     │
    │     ├─ matchRules(ev, "request")
    │     │    · 过滤 stage=="request" 的规则
    │     │    · 基于 ev.Request 数据匹配条件
    │     ├─ 命中 rule-001 (stage:request) → setHeader, block, setUrl ...
    │     └─ ContinueRequest / FulfillRequest
    │
    │  ... 服务器处理 ...
    │
    └─▶ Fetch.RequestPaused (RequestID=A, ResponseStatusCode=200)
          │
          ├─ matchRules(ev, "response")    ← 重新匹配，但基于同样的 ev.Request 数据
          │    · 过滤 stage=="response" 的规则
          │    · 基于 ev.Request 数据匹配条件（与请求阶段数据一致）
          ├─ 命中 rule-003 (stage:response) → setBody, replaceBodyText ...
          └─ ContinueResponse / FulfillRequest
```

---

## 匹配条件数据源

所有条件均基于 `ev.Request`，不受 stage 影响：

| 条件 | 数据源 |
|------|--------|
| URL 条件（5种） | `ev.Request.URL` |
| method | `ev.Request.Method` |
| resourceType | `ev.ResourceType` |
| header 条件（5种） | `ev.Request.Headers`（请求头） |
| query 条件（5种） | 从 `ev.Request.URL` 解析 |
| cookie 条件（5种） | 从 `ev.Request.Headers["Cookie"]` 解析 |
| body 条件（3种） | `ev.Request.PostData`（请求体） |

> bodyContains/bodyRegex/bodyJsonPath 匹配的是**请求体**，仅对 POST/PUT 等有 Body 的请求有效。

---

## 实现现状

当前代码已正确实现此模型，无需改动：

```go
func (i *Intercept) processEvent(ctx context.Context, ev *fetch.RequestPausedReply) {
    stage := "request"
    if ev.ResponseStatusCode != nil {
        stage = "response"
    }

    rule := i.matchRules(ev, stage)  // stage 过滤 + ev.Request 数据匹配
    if rule == nil {
        i.continueEvent(ctx, ev.RequestID, stage, nil)
        return
    }
    i.executeActions(ctx, ev, rule, stage)  // stage 传递给 action 校验
}
```

`continueEvent` 在请求阶段调用 `ContinueRequest`，响应阶段调用 `ContinueResponse`。`executeActions` 中响应专属 action（setStatus/setBody/replaceBodyText/patchBodyJson）已校验 `stage != "response"` 时跳过。
