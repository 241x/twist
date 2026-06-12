package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"

	"github.com/241x/twist/internal/log"
)

type Intercept struct {
	cdp         *CDP
	config      *Config
	workerCount int
}

func NewIntercept(cdp *CDP, config *Config) *Intercept {
	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}

	return &Intercept{
		cdp:         cdp,
		config:      config,
		workerCount: workers,
	}
}

func (i *Intercept) Start(ctx context.Context) error {
	if err := i.cdp.EnableNetwork(ctx); err != nil {
		return err
	}

	paused, err := i.cdp.EnableFetch(ctx)
	if err != nil {
		return err
	}
	defer paused.Close()

	logger := log.FromContext(ctx)

	workerCh := make(chan *fetch.RequestPausedReply, 100)

	var wg sync.WaitGroup
	for j := 0; j < i.workerCount; j++ {
		wg.Add(1)
		go i.worker(ctx, workerCh, &wg)
	}

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

		if i.shouldBypass(ctx, ev) {
			logger.Debug().
				Str("url", ev.Request.URL).
				Str("reason", "bypass").
				Msg("request bypassed")
			i.continueRequest(ctx, ev.RequestID, nil)
			continue
		}

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
	defer func() {
		if r := recover(); r != nil {
			log.FromContext(ctx).Error().Interface("panic", r).Msg("worker panic recovered")
		}
	}()

	for ev := range ch {
		i.processEvent(ctx, ev)
	}
}

func (i *Intercept) processEvent(ctx context.Context, ev *fetch.RequestPausedReply) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	logger := log.FromContext(ctx)

	rule := i.matchRules(ev, "request")
	if rule == nil {
		i.continueRequest(ctx, ev.RequestID, nil)
		return
	}

	logger.Debug().
		Str("rule", rule.ID).
		Str("url", ev.Request.URL).
		Msg("rule matched")

	i.executeActions(ctx, ev, rule)
}

func (i *Intercept) shouldBypass(ctx context.Context, ev *fetch.RequestPausedReply) bool {
	u := ev.Request.URL

	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return true
	}

	if ev.ResourceType.String() == "WebSocket" {
		return true
	}

	if ev.Request.Method == "OPTIONS" {
		return true
	}

	cdpHost := i.cdp.host
	cdpPort := strconv.Itoa(i.cdp.port)
	if strings.Contains(u, cdpHost+":"+cdpPort) {
		return true
	}

	return false
}

func (i *Intercept) continueRequest(ctx context.Context, requestID fetch.RequestID, headers []fetch.HeaderEntry) {
	args := fetch.NewContinueRequestArgs(requestID)
	if len(headers) > 0 {
		args.SetHeaders(headers)
	}

	if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
		log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("continue request failed")
	}
}

func (i *Intercept) continueRequestURL(ctx context.Context, requestID fetch.RequestID, newURL string) {
	args := fetch.NewContinueRequestArgs(requestID)
	args.SetURL(newURL)

	if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
		log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("continue request with URL failed")
	}
}

func (i *Intercept) matchRules(ev *fetch.RequestPausedReply, stage string) *Rule {
	rules := i.config.Rules
	if len(rules) == 0 {
		return nil
	}

	enabled := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.Enabled && r.Stage == stage {
			enabled = append(enabled, r)
		}
	}

	sortByPriority(enabled)

	for _, rule := range enabled {
		if i.matchRule(ev, &rule) {
			return &rule
		}
	}

	return nil
}

func (i *Intercept) matchRule(ev *fetch.RequestPausedReply, rule *Rule) bool {
	match := rule.Match

	if len(match.AllOf) > 0 {
		for _, cond := range match.AllOf {
			if !i.matchCondition(ev, cond) {
				return false
			}
		}
	}

	if len(match.AnyOf) > 0 {
		found := false
		for _, cond := range match.AnyOf {
			if i.matchCondition(ev, cond) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (i *Intercept) matchCondition(ev *fetch.RequestPausedReply, cond Condition) bool {
	hdrs := parseHeaders(ev.Request.Headers)

	switch cond.Type {
	case "urlEquals":
		return ev.Request.URL == cond.Value
	case "urlPrefix":
		return strings.HasPrefix(ev.Request.URL, cond.Value)
	case "urlSuffix":
		return strings.HasSuffix(ev.Request.URL, cond.Value)
	case "urlContains":
		return strings.Contains(ev.Request.URL, cond.Value)
	case "urlRegex":
		return matchRegex(cond.Pattern, ev.Request.URL)
	case "method":
		for _, v := range cond.Values {
			if strings.EqualFold(v, ev.Request.Method) {
				return true
			}
		}
		return false
	case "resourceType":
		rt := ev.ResourceType.String()
		for _, v := range cond.Values {
			if strings.EqualFold(v, rt) {
				return true
			}
		}
		return false
	case "headerExists":
		return headerHasKey(hdrs, cond.Name)
	case "headerNotExists":
		return !headerHasKey(hdrs, cond.Name)
	case "headerEquals":
		val := headerGet(hdrs, cond.Name)
		return val == cond.Value
	case "headerContains":
		val := headerGet(hdrs, cond.Name)
		return strings.Contains(val, cond.Value)
	case "headerRegex":
		val := headerGet(hdrs, cond.Name)
		return val != "" && matchRegex(cond.Pattern, val)
	case "cookieExists", "cookieNotExists", "cookieEquals", "cookieContains", "cookieRegex":
		cookies := parseCookies(hdrs)
		return matchCookie(cond, cookies)
	case "queryExists", "queryNotExists", "queryEquals", "queryContains", "queryRegex":
		queries := parseQuery(ev.Request.URL)
		return matchQuery(cond, queries)
	case "bodyContains", "bodyRegex", "bodyJsonPath":
		return false
	default:
		return false
	}
}

func matchCookie(cond Condition, cookies map[string]string) bool {
	switch cond.Type {
	case "cookieExists":
		_, ok := cookies[cond.Name]
		return ok
	case "cookieNotExists":
		_, ok := cookies[cond.Name]
		return !ok
	case "cookieEquals":
		val, ok := cookies[cond.Name]
		return ok && val == cond.Value
	case "cookieContains":
		val, ok := cookies[cond.Name]
		return ok && strings.Contains(val, cond.Value)
	case "cookieRegex":
		val, ok := cookies[cond.Name]
		return ok && matchRegex(cond.Pattern, val)
	}
	return false
}

func matchQuery(cond Condition, queries map[string]string) bool {
	switch cond.Type {
	case "queryExists":
		_, ok := queries[cond.Name]
		return ok
	case "queryNotExists":
		_, ok := queries[cond.Name]
		return !ok
	case "queryEquals":
		val, ok := queries[cond.Name]
		return ok && val == cond.Value
	case "queryContains":
		val, ok := queries[cond.Name]
		return ok && strings.Contains(val, cond.Value)
	case "queryRegex":
		val, ok := queries[cond.Name]
		return ok && matchRegex(cond.Pattern, val)
	}
	return false
}

func parseCookies(headers map[string]string) map[string]string {
	val := headerGet(headers, "Cookie")
	if val == "" {
		return nil
	}

	cookies := make(map[string]string)
	pairs := strings.Split(val, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx < 0 {
			cookies[pair] = ""
		} else {
			cookies[pair[:idx]] = pair[idx+1:]
		}
	}
	return cookies
}

func parseQuery(rawURL string) map[string]string {
	queries := make(map[string]string)
	idx := strings.Index(rawURL, "?")
	if idx < 0 {
		return queries
	}

	raw := rawURL[idx+1:]
	fragIdx := strings.Index(raw, "#")
	if fragIdx >= 0 {
		raw = raw[:fragIdx]
	}

	pairs := strings.Split(raw, "&")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if parts[0] == "" {
			continue
		}
		k, _ := url.QueryUnescape(parts[0])
		if len(parts) == 2 {
			v, _ := url.QueryUnescape(parts[1])
			queries[k] = v
		} else {
			queries[k] = ""
		}
	}
	return queries
}

func parseHeaders(raw network.Headers) map[string]string {
	result := make(map[string]string)
	if len(raw) == 0 {
		return result
	}

	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return result
	}

	for k, v := range m {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

func headerHasKey(headers map[string]string, name string) bool {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

func headerGet(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func matchRegex(pattern, s string) bool {
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

func (i *Intercept) executeActions(ctx context.Context, ev *fetch.RequestPausedReply, rule *Rule) {
	logger := log.FromContext(ctx)

	for _, action := range rule.Actions {
		switch action.Type {
		case "block":
			var statusCode int
			if action.StatusCode > 0 {
				statusCode = action.StatusCode
			} else {
				statusCode = 200
			}

			args := fetch.NewFulfillRequestArgs(ev.RequestID, statusCode)

			if len(action.Headers) > 0 {
				headers := make([]fetch.HeaderEntry, 0, len(action.Headers))
				for k, v := range action.Headers {
					headers = append(headers, fetch.HeaderEntry{Name: k, Value: v})
				}
				args.SetResponseHeaders(headers)
			}

			if action.Body != "" {
				args.SetBody([]byte(action.Body))
			}

			logger.Debug().
				Str("rule", rule.ID).
				Int("statusCode", statusCode).
				Msg("block request")

			if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("fulfill request failed")
			}
			return

		case "setHeader":
			logger.Debug().
				Str("rule", rule.ID).
				Str("header", action.Name).
				Msg("set header")

			headers := []fetch.HeaderEntry{
				{Name: action.Name, Value: fmt.Sprintf("%v", action.Value)},
			}
			i.continueRequest(ctx, ev.RequestID, headers)
			return

		case "removeHeader":
			logger.Debug().
				Str("rule", rule.ID).
				Str("header", action.Name).
				Msg("remove header")
			i.continueRequest(ctx, ev.RequestID, nil)
			return

		case "setUrl":
			logger.Debug().
				Str("rule", rule.ID).
				Str("url", fmt.Sprintf("%v", action.Value)).
				Msg("set url")
			args := fetch.NewContinueRequestArgs(ev.RequestID)
			args.SetURL(fmt.Sprintf("%v", action.Value))
			if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("set url failed")
			}
			return

		case "setMethod":
			logger.Debug().
				Str("rule", rule.ID).
				Str("method", fmt.Sprintf("%v", action.Value)).
				Msg("set method")
			args := fetch.NewContinueRequestArgs(ev.RequestID)
			args.SetMethod(fmt.Sprintf("%v", action.Value))
			if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("set method failed")
			}
			return

		case "setQueryParam":
			newURL := setQueryParamValue(ev.Request.URL, action.Name, fmt.Sprintf("%v", action.Value))
			logger.Debug().
				Str("rule", rule.ID).
				Str("param", action.Name).
				Msg("set query param")
			i.continueRequestURL(ctx, ev.RequestID, newURL)
			return

		case "removeQueryParam":
			newURL := removeQueryParamValue(ev.Request.URL, action.Name)
			logger.Debug().
				Str("rule", rule.ID).
				Str("param", action.Name).
				Msg("remove query param")
			i.continueRequestURL(ctx, ev.RequestID, newURL)
			return

		case "setCookie":
			logger.Debug().
				Str("rule", rule.ID).
				Str("cookie", action.Name).
				Msg("set cookie")
			i.continueRequest(ctx, ev.RequestID, nil)
			return

		case "removeCookie":
			logger.Debug().
				Str("rule", rule.ID).
				Str("cookie", action.Name).
				Msg("remove cookie")
			i.continueRequest(ctx, ev.RequestID, nil)
			return

		case "setFormField":
			logger.Debug().
				Str("rule", rule.ID).
				Str("field", action.Name).
				Msg("set form field")
			i.continueRequest(ctx, ev.RequestID, nil)
			return

		case "removeFormField":
			logger.Debug().
				Str("rule", rule.ID).
				Str("field", action.Name).
				Msg("remove form field")
			i.continueRequest(ctx, ev.RequestID, nil)
			return

		default:
			logger.Warn().
				Str("rule", rule.ID).
				Str("action", action.Type).
				Msg("unsupported action, passing through")

			i.continueRequest(ctx, ev.RequestID, nil)
			return
		}
	}
}

func sortByPriority(rules []Rule) {
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

func setQueryParamValue(rawURL, name, value string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set(name, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func removeQueryParamValue(rawURL, name string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Del(name)
	u.RawQuery = q.Encode()
	return u.String()
}
