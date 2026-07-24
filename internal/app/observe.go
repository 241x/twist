// Package app 提供 twist 的核心业务逻辑：浏览器管理、CDP 连接、规则引擎、网络观测和请求拦截。
package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"

	"github.com/241x/twist/internal/log"
)

const observeBodyLimit = 4 * 1024 // 默认响应体截断长度

// ObserveEvent 单条观测事件，序列化为 JSONL 输出。
type ObserveEvent struct {
	Type             string            `json:"type"`
	RequestID        string            `json:"requestId"`
	URL              string            `json:"url,omitempty"`
	Method           string            `json:"method,omitempty"`
	ResourceType     string            `json:"resourceType,omitempty"`
	RequestHeaders   map[string]string `json:"requestHeaders,omitempty"`
	PostData         string            `json:"postData,omitempty"`
	StatusCode       int               `json:"statusCode,omitempty"`
	StatusText       string            `json:"statusText,omitempty"`
	ResponseHeaders  map[string]string `json:"responseHeaders,omitempty"`
	Body             string            `json:"body,omitempty"`
	BodyTruncated    bool              `json:"bodyTruncated,omitempty"`
	BodySize         int               `json:"bodySize,omitempty"`
	ErrorReason      string            `json:"errorReason,omitempty"`
}

// ObserveOptions 观测模式参数。
type ObserveOptions struct {
	Enabled  bool
	Count    int
	Duration time.Duration
	FullBody bool
	Filter   ObserveFilter
}

// ObserveFilter 观测事件过滤条件，多个字段之间 AND，单字段内逗号分隔的值之间 OR。
type ObserveFilter struct {
	URLs  []string
	Types []string
}

// IsEmpty 判断过滤条件是否为空。
func (f ObserveFilter) IsEmpty() bool { return len(f.URLs) == 0 && len(f.Types) == 0 }

// Match 判断给定 URL 和资源类型是否通过过滤。
func (f ObserveFilter) Match(url, resourceType string) bool {
	if len(f.URLs) > 0 {
		matched := false
		for _, p := range f.URLs {
			if strings.Contains(url, p) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(f.Types) > 0 {
		matched := false
		for _, t := range f.Types {
			if strings.EqualFold(resourceType, t) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Observe 观测引擎：接收 Fetch 事件 → 提取数据 → 写入 stdout JSONL。
type Observe struct {
	cdp  *CDP
	opts ObserveOptions
}

func NewObserve(cdp *CDP, opts ObserveOptions) *Observe {
	return &Observe{cdp: cdp, opts: opts}
}

// Start 启动观测循环。
func (o *Observe) Start(ctx context.Context) error {
	if err := o.cdp.EnableNetwork(ctx); err != nil {
		return err
	}

	paused, err := o.cdp.EnableFetch(ctx)
	if err != nil {
		return err
	}
	defer paused.Close()

	var deadline <-chan time.Time
	if o.opts.Duration > 0 {
		timer := time.NewTimer(o.opts.Duration)
		defer timer.Stop()
		deadline = timer.C
	}

	seen := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return nil
		default:
		}

		ev, err := paused.Recv()
		if err != nil {
			return err
		}

		stage := "request"
		if ev.ResponseStatusCode != nil || ev.ResponseErrorReason != nil {
			stage = "response"
		}

		if o.shouldBypass(ev) {
			o.continueEvent(ctx, ev.RequestID, stage)
			continue
		}

		if !o.opts.Filter.IsEmpty() {
			url := ev.Request.URL
			rt := ev.ResourceType.String()
			if !o.opts.Filter.Match(url, rt) {
				o.continueEvent(ctx, ev.RequestID, stage)
				continue
			}
		}

		obs := o.buildEvent(ctx, ev, stage)

		data, err := json.Marshal(obs)
		if err != nil {
			log.FromContext(ctx).Error().Err(err).Str("url", ev.Request.URL).Msg("observe marshal failed")
			o.continueEvent(ctx, ev.RequestID, stage)
			continue
		}

		fmt.Fprintln(os.Stdout, string(data))

		o.continueEvent(ctx, ev.RequestID, stage)
		seen++

		if o.opts.Count > 0 && seen >= o.opts.Count {
			return nil
		}
	}
}

func (o *Observe) shouldBypass(ev *fetch.RequestPausedReply) bool {
	u := ev.Request.URL
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return true
	}
	if ev.ResourceType.String() == "WebSocket" {
		return true
	}
	return false
}

func (o *Observe) buildEvent(ctx context.Context, ev *fetch.RequestPausedReply, stage string) ObserveEvent {
	req := ev.Request
	obs := ObserveEvent{
		Type:         stage,
		RequestID:    string(ev.RequestID),
		URL:          req.URL,
		ResourceType: ev.ResourceType.String(),
	}

	if stage == "request" {
		obs.Method = req.Method
		obs.RequestHeaders = parseHeaders(req.Headers)
		if req.HasPostData != nil && *req.HasPostData {
			obs.PostData = getPostDataStr(req)
		}
	} else {
		if ev.ResponseErrorReason != nil {
			obs.ErrorReason = string(*ev.ResponseErrorReason)
		}
		if ev.ResponseStatusCode != nil {
			obs.StatusCode = *ev.ResponseStatusCode
		}
		if ev.ResponseStatusText != nil {
			obs.StatusText = *ev.ResponseStatusText
		}
		obs.ResponseHeaders = parseHeadersFromEntries(ev.ResponseHeaders)

		if ev.ResponseErrorReason == nil {
			body, truncated, bodySize := o.getBody(ctx, ev.RequestID)
			obs.Body = body
			obs.BodyTruncated = truncated
			obs.BodySize = bodySize
		}
	}

	return obs
}

func (o *Observe) getBody(ctx context.Context, requestID fetch.RequestID) (body string, truncated bool, totalSize int) {
	reply, err := o.cdp.TargetClient().Fetch.GetResponseBody(ctx, fetch.NewGetResponseBodyArgs(requestID))
	if err != nil {
		return "", false, 0
	}

	raw := reply.Body
	if reply.Base64Encoded {
		if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
			raw = string(decoded)
		}
	}

	limit := observeBodyLimit
	if o.opts.FullBody {
		limit = len(raw)
	}

	if len(raw) > limit {
		return raw[:limit], true, len(raw)
	}

	return raw, false, len(raw)
}

func (o *Observe) continueEvent(ctx context.Context, requestID fetch.RequestID, stage string) {
	if stage == "response" {
		args := fetch.NewContinueResponseArgs(requestID)
		if err := o.cdp.TargetClient().Fetch.ContinueResponse(ctx, args); err != nil {
			log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("observe continue response failed")
		}
	} else {
		args := fetch.NewContinueRequestArgs(requestID)
		if err := o.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
			log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("observe continue request failed")
		}
	}
}

func parseHeadersFromEntries(entries []fetch.HeaderEntry) map[string]string {
	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Name] = e.Value
	}
	return result
}
