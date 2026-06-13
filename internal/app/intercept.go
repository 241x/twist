package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mafredri/cdp/protocol/fetch"
	"github.com/mafredri/cdp/protocol/network"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/241x/twist/internal/log"
)

var _ struct {
	json.Number
	gjson.Result
	sjson.Options
}

const maxBodySize = 5 * 1024 * 1024
const maxResponseBodySize = 10 * 1024 * 1024

var regexCache sync.Map

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

		stage := "request"
		if ev.ResponseStatusCode != nil {
			stage = "response"
		}

		if i.shouldBypass(ctx, ev) {
			logger.Debug().
				Str("url", ev.Request.URL).
				Str("stage", stage).
				Str("reason", "bypass").
				Msg("request bypassed")
			i.continueEvent(ctx, ev.RequestID, stage, nil)
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

	stage := "request"
	if ev.ResponseStatusCode != nil {
		stage = "response"
	}

	logger := log.FromContext(ctx)

	rule := i.matchRules(ev, stage)
	if rule == nil {
		i.continueEvent(ctx, ev.RequestID, stage, nil)
		return
	}

	logger.Debug().
		Str("rule", rule.ID).
		Str("url", ev.Request.URL).
		Str("stage", stage).
		Msg("rule matched")

	i.executeActions(ctx, ev, rule, stage)
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

	hdrs := parseHeaders(ev.Request.Headers)
	if cl := headerGet(hdrs, "Content-Length"); cl != "" {
		if n, err := strconv.Atoi(cl); err == nil && n > maxBodySize {
			return true
		}
	}

	return false
}

func (i *Intercept) continueEvent(ctx context.Context, requestID fetch.RequestID, stage string, headers []fetch.HeaderEntry) {
	if stage == "response" {
		if len(headers) > 0 {
			args := fetch.NewFulfillRequestArgs(requestID, 200)
			args.SetResponseHeaders(headers)
			if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
				log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("fulfill response failed")
			}
			return
		}
		args := fetch.NewContinueResponseArgs(requestID)
		if err := i.cdp.TargetClient().Fetch.ContinueResponse(ctx, args); err != nil {
			log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("continue response failed")
		}
	} else {
		i.continueRequest(ctx, requestID, headers)
	}
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

func (i *Intercept) continueRequestPost(ctx context.Context, requestID fetch.RequestID, postData []byte) {
	args := fetch.NewContinueRequestArgs(requestID)
	args.SetPostData(postData)

	if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
		log.FromContext(ctx).Error().Err(err).Str("requestID", string(requestID)).Msg("continue request with post data failed")
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
		body := getPostDataStr(ev.Request)
		return matchBody(cond, body)
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

	re, ok := regexCache.Load(pattern)
	if !ok {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return false
		}
		regexCache.Store(pattern, re)
	}

	return re.(*regexp.Regexp).MatchString(s)
}

func (i *Intercept) executeActions(ctx context.Context, ev *fetch.RequestPausedReply, rule *Rule, stage string) {
	logger := log.FromContext(ctx)
	hdrs := parseHeaders(ev.Request.Headers)

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
			if stage == "response" {
				newHeaders := modifyResponseCookie(ev.ResponseHeaders, action.Name, fmt.Sprintf("%v", action.Value))
				logger.Debug().Str("rule", rule.ID).Str("cookie", action.Name).Msg("set response cookie")
				i.continueEvent(ctx, ev.RequestID, stage, newHeaders)
				return
			}
			newHeaders := modifyCookieHeader(hdrs, action.Name, fmt.Sprintf("%v", action.Value))
			logger.Debug().Str("rule", rule.ID).Str("cookie", action.Name).Msg("set cookie")
			i.continueRequest(ctx, ev.RequestID, newHeaders)
			return

		case "removeCookie":
			if stage == "response" {
				newHeaders := removeResponseCookie(ev.ResponseHeaders, action.Name)
				logger.Debug().Str("rule", rule.ID).Str("cookie", action.Name).Msg("remove response cookie")
				i.continueEvent(ctx, ev.RequestID, stage, newHeaders)
				return
			}
			newHeaders := removeCookieFromHeader(hdrs, action.Name)
			logger.Debug().Str("rule", rule.ID).Str("cookie", action.Name).Msg("remove cookie")
			i.continueRequest(ctx, ev.RequestID, newHeaders)
			return

		case "setFormField":
			contentType := headerGet(hdrs, "Content-Type")
			if strings.Contains(contentType, "multipart/form-data") {
				boundary := extractBoundary(contentType)
				if boundary == "" {
					logger.Warn().Msg("setFormField: cannot extract multipart boundary")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				body, ok := getPostData(ev.Request)
				if !ok {
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				newBody, err := setMultipartField(body, boundary, action.Name, fmt.Sprintf("%v", action.Value))
				if err != nil {
					logger.Error().Err(err).Msg("setFormField: multipart modify failed")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				logger.Debug().Str("rule", rule.ID).Str("field", action.Name).Msg("set multipart form field")
				i.continueRequestPost(ctx, ev.RequestID, newBody)
				return
			}
			body, ok := getPostData(ev.Request)
			if ok {
				body = setFormFieldValue(body, action.Name, fmt.Sprintf("%v", action.Value))
			}
			logger.Debug().Str("rule", rule.ID).Str("field", action.Name).Msg("set form field")
			i.continueRequestPost(ctx, ev.RequestID, body)
			return

		case "removeFormField":
			contentType := headerGet(hdrs, "Content-Type")
			if strings.Contains(contentType, "multipart/form-data") {
				boundary := extractBoundary(contentType)
				if boundary == "" {
					logger.Warn().Msg("removeFormField: cannot extract multipart boundary")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				body, ok := getPostData(ev.Request)
				if !ok {
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				newBody, err := removeMultipartField(body, boundary, action.Name)
				if err != nil {
					logger.Error().Err(err).Msg("removeFormField: multipart remove failed")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				logger.Debug().Str("rule", rule.ID).Str("field", action.Name).Msg("remove multipart form field")
				i.continueRequestPost(ctx, ev.RequestID, newBody)
				return
			}
			body, ok := getPostData(ev.Request)
			if ok {
				body = removeFormFieldValue(body, action.Name)
			}
			logger.Debug().Str("rule", rule.ID).Str("field", action.Name).Msg("remove form field")
			i.continueRequestPost(ctx, ev.RequestID, body)
			return

		case "setStatus":
			if stage != "response" {
				logger.Warn().Str("action", "setStatus").Msg("setStatus only valid in response stage, passing through")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			args := fetch.NewFulfillRequestArgs(ev.RequestID, action.StatusCode)
			logger.Debug().
				Str("rule", rule.ID).
				Int("status", action.StatusCode).
				Msg("set status")
			if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("fulfill request failed")
			}
			return

		case "setBody":
			body := action.Body
			if body == "" {
				body = fmt.Sprintf("%v", action.Value)
			}

			if stage == "response" {
				logger.Debug().Str("rule", rule.ID).Int("bodyLen", len(body)).Msg("set response body")
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(body))
				if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
					logger.Error().Err(err).Msg("fulfill request failed")
				}
				return
			}

			headers := []fetch.HeaderEntry{}
			contentType := headerGet(hdrs, "Content-Type")
			if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
				if !strings.Contains(contentType, "json") {
					headers = append(headers, fetch.HeaderEntry{Name: "Content-Type", Value: "application/json"})
					logger.Debug().Str("rule", rule.ID).Msg("set Content-Type to application/json")
				}
			}

			logger.Debug().Str("rule", rule.ID).Int("bodyLen", len(body)).Str("setBody", body).Msg("set request body")
			args := fetch.NewContinueRequestArgs(ev.RequestID)
			args.SetPostData([]byte(body))
			if len(headers) > 0 {
				args.SetHeaders(headers)
			}
			if err := i.cdp.TargetClient().Fetch.ContinueRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("set body failed")
			}
			return

		case "appendBody":
			appendContent := fmt.Sprintf("%v", action.Value)

			if stage == "response" {
				body, err := i.getResponseBody(ctx, ev.RequestID)
				if err != nil {
					logger.Error().Err(err).Msg("failed to get response body for append")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				newBody := body + appendContent
				logger.Debug().Str("rule", rule.ID).Int("origLen", len(body)).Int("newLen", len(newBody)).Msg("append response body")
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(newBody))
				if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
					logger.Error().Err(err).Msg("fulfill request failed")
				}
				return
			}

			postData, ok := getPostData(ev.Request)
			origBody := ""
			if ok {
				origBody = string(postData)
			}
			newBody := origBody + appendContent
			logger.Debug().Str("rule", rule.ID).Int("origLen", len(origBody)).Int("newLen", len(newBody)).Msg("append request body")
			i.continueRequestPost(ctx, ev.RequestID, []byte(newBody))
			return

		case "replaceBodyText":
			if stage == "response" {
				body, err := i.getResponseBody(ctx, ev.RequestID)
				if err != nil {
					logger.Error().Err(err).Msg("failed to get response body")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				if action.ReplaceAll {
					body = strings.ReplaceAll(body, action.Search, action.Replace)
				} else {
					body = strings.Replace(body, action.Search, action.Replace, 1)
				}
				logger.Debug().Str("rule", rule.ID).Str("search", action.Search).Msg("replace response body text")
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(body))
				if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
					logger.Error().Err(err).Msg("fulfill request failed")
				}
				return
			}

			postData, ok := getPostData(ev.Request)
			if !ok {
				logger.Warn().Msg("replaceBodyText: no request body")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			body := string(postData)
			if action.ReplaceAll {
				body = strings.ReplaceAll(body, action.Search, action.Replace)
			} else {
				body = strings.Replace(body, action.Search, action.Replace, 1)
			}
			logger.Debug().Str("rule", rule.ID).Str("search", action.Search).Msg("replace request body text")
			i.continueRequestPost(ctx, ev.RequestID, []byte(body))
			return

		case "patchBodyJson":
			if stage == "response" {
				body, err := i.getResponseBody(ctx, ev.RequestID)
				if err != nil {
					logger.Error().Err(err).Msg("failed to get response body")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				patched, err := applyJSONPatch(body, action.Patches)
				if err != nil {
					logger.Error().Err(err).Msg("json patch failed")
					i.continueEvent(ctx, ev.RequestID, stage, nil)
					return
				}
				args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
				args.SetBody([]byte(patched))
				if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
					logger.Error().Err(err).Msg("fulfill request failed")
				}
				return
			}

			postData, ok := getPostData(ev.Request)
			if !ok {
				logger.Warn().Msg("patchBodyJson: no request body to patch")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			origBody := string(postData)
			logger.Debug().Str("rule", rule.ID).Int("origLen", len(origBody)).Str("origBody", origBody).Msg("patchBodyJson before")
			patched, err := applyJSONPatch(origBody, action.Patches)
			if err != nil {
				logger.Error().Err(err).Msg("json patch failed on request body")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			logger.Debug().Str("rule", rule.ID).Int("patchedLen", len(patched)).Str("patchedBody", patched).Msg("patchBodyJson after")
			i.continueRequestPost(ctx, ev.RequestID, []byte(patched))
			return

		case "replaceElement":
			if stage != "response" {
				logger.Warn().Str("action", "replaceElement").Msg("replaceElement only valid in response stage")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			body, err := i.getResponseBody(ctx, ev.RequestID)
			if err != nil {
				logger.Warn().Err(err).Str("selector", action.Selector).Msg("replaceElement: body not available, passing through")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
			if err != nil {
				logger.Error().Err(err).Msg("failed to parse HTML for replaceElement")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			doc.Find(action.Selector).SetHtml(fmt.Sprintf("%v", action.Value))
			newBody, err := doc.Html()
			if err != nil {
				logger.Error().Err(err).Msg("failed to serialize HTML after replaceElement")
				i.continueEvent(ctx, ev.RequestID, stage, nil)
				return
			}
			logger.Debug().Str("rule", rule.ID).Str("selector", action.Selector).Msg("replace element")
			args := fetch.NewFulfillRequestArgs(ev.RequestID, 200)
			args.SetBody([]byte(newBody))
			if err := i.cdp.TargetClient().Fetch.FulfillRequest(ctx, args); err != nil {
				logger.Error().Err(err).Msg("fulfill request failed")
			}
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

func modifyCookieHeader(headers map[string]string, name, value string) []fetch.HeaderEntry {
	cookieVal := headerGet(headers, "Cookie")
	pairs := parseCookiePairs(cookieVal)
	pairs[name] = value

	newCookie := buildCookieString(pairs)
	entries := buildHeaderEntries(headers, "Cookie", newCookie)
	return entries
}

func removeCookieFromHeader(headers map[string]string, name string) []fetch.HeaderEntry {
	cookieVal := headerGet(headers, "Cookie")
	pairs := parseCookiePairs(cookieVal)
	delete(pairs, name)

	newCookie := buildCookieString(pairs)
	entries := buildHeaderEntries(headers, "Cookie", newCookie)
	return entries
}

func parseCookiePairs(cookieStr string) map[string]string {
	pairs := make(map[string]string)
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		pairs[k] = v
	}
	return pairs
}

func buildCookieString(pairs map[string]string) string {
	parts := make([]string, 0, len(pairs))
	for k, v := range pairs {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func buildHeaderEntries(headers map[string]string, skipKey, newValue string) []fetch.HeaderEntry {
	entries := make([]fetch.HeaderEntry, 0, len(headers)+1)
	cookieReplaced := false
	for k, v := range headers {
		if strings.EqualFold(k, skipKey) {
			if newValue != "" {
				entries = append(entries, fetch.HeaderEntry{Name: k, Value: newValue})
				cookieReplaced = true
			}
			continue
		}
		if v != "" {
			entries = append(entries, fetch.HeaderEntry{Name: k, Value: v})
		}
	}
	if !cookieReplaced && newValue != "" {
		entries = append(entries, fetch.HeaderEntry{Name: "Cookie", Value: newValue})
	}
	return entries
}

func getPostData(req network.Request) ([]byte, bool) {
	s := getPostDataStr(req)
	if s == "" {
		return nil, false
	}
	return []byte(s), true
}

func getPostDataStr(req network.Request) string {
	if req.HasPostData != nil && *req.HasPostData && len(req.PostDataEntries) > 0 {
		var parts []string
		for _, entry := range req.PostDataEntries {
			if entry.Bytes != nil {
				raw := *entry.Bytes
				if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
					parts = append(parts, string(decoded))
				} else {
					parts = append(parts, raw)
				}
			}
		}
		return strings.Join(parts, "")
	}
	if req.PostData != nil {
		return *req.PostData
	}
	return ""
}

func matchBody(cond Condition, body string) bool {
	if body == "" {
		return false
	}

	switch cond.Type {
	case "bodyContains":
		return strings.Contains(body, cond.Value)
	case "bodyRegex":
		return matchRegex(cond.Pattern, body)
	case "bodyJsonPath":
		return matchBodyJsonPath(body, cond.Path, cond.Value)
	}
	return false
}

func matchBodyJsonPath(body, path, expected string) bool {
	result := gjson.Get(body, toGJSONPath(path))
	if !result.Exists() {
		return false
	}
	return fmt.Sprintf("%v", result.Value()) == expected
}

func setFormFieldValue(body []byte, name, value string) []byte {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return body
	}
	vals.Set(name, value)
	return []byte(vals.Encode())
}

func removeFormFieldValue(body []byte, name string) []byte {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return body
	}
	vals.Del(name)
	return []byte(vals.Encode())
}

func extractBoundary(contentType string) string {
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			b := strings.TrimPrefix(part, "boundary=")
			b = strings.Trim(b, "\"")
			return b
		}
	}
	return ""
}

func setMultipartField(body []byte, boundary, fieldName, newValue string) ([]byte, error) {
	return modifyMultipart(body, boundary, fieldName, func(lines []string) ([]string, bool) {
		for i, line := range lines {
			if strings.Contains(line, `name="`+fieldName+`"`) {
				for j := i + 1; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) == "" && j+1 < len(lines) {
						lines[j+1] = newValue
						return lines, true
					}
				}
			}
		}
		return lines, false
	})
}

func removeMultipartField(body []byte, boundary, fieldName string) ([]byte, error) {
	return modifyMultipart(body, boundary, fieldName, func(lines []string) ([]string, bool) {
		start := -1
		end := -1
		for i, line := range lines {
			if start == -1 && strings.Contains(line, `name="`+fieldName+`"`) {
				start = i - 1
			}
			if start != -1 && strings.HasPrefix(line, "--"+boundary) && i > start+1 {
				end = i
				break
			}
		}
		if start >= 0 && end > start {
			for i := start; i < end; i++ {
				lines[i] = ""
			}
			return lines, true
		}
		return lines, false
	})
}

func modifyMultipart(body []byte, boundary, fieldName string, fn func([]string) ([]string, bool)) ([]byte, error) {
	text := string(body)
	lines := strings.Split(text, "\r\n")
	lines, _ = fn(lines)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return []byte(strings.Join(result, "\r\n")), nil
}

func (i *Intercept) getResponseBody(ctx context.Context, requestID fetch.RequestID) (string, error) {
	reply, err := i.cdp.TargetClient().Fetch.GetResponseBody(ctx, fetch.NewGetResponseBodyArgs(requestID))
	if err != nil {
		return "", err
	}
	if len(reply.Body) > maxResponseBodySize {
		return "", fmt.Errorf("response body too large: %d bytes", len(reply.Body))
	}
	body := reply.Body
	if reply.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err == nil {
			body = string(decoded)
		}
	}
	return body, nil
}

func applyJSONPatch(body string, patches []JSONPatch) (string, error) {
	var err error
	result := body

	for _, p := range patches {
		path := toGJSONPath(p.Path)
		from := toGJSONPath(p.From)

		switch p.Op {
		case "add", "replace":
			result, err = sjson.Set(result, path, p.Value)
		case "remove":
			result, err = sjson.Delete(result, path)
		case "move":
			val := gjson.Get(result, from)
			if !val.Exists() {
				err = fmt.Errorf("move: source path %q not found", p.From)
				break
			}
			result, err = sjson.Delete(result, from)
			if err != nil {
				break
			}
			result, err = sjson.Set(result, path, val.Value())
		case "copy":
			val := gjson.Get(result, from)
			if !val.Exists() {
				err = fmt.Errorf("copy: source path %q not found", p.From)
				break
			}
			result, err = sjson.Set(result, path, val.Value())
		case "test":
			val := gjson.Get(result, path)
			if !val.Exists() {
				err = fmt.Errorf("test: path %q not found", p.Path)
				break
			}
			if fmt.Sprintf("%v", val.Value()) != fmt.Sprintf("%v", p.Value) {
				err = fmt.Errorf("test failed: %v != %v", val.Value(), p.Value)
			}
		default:
			err = fmt.Errorf("unknown patch op: %q", p.Op)
		}
		if err != nil {
			return "", err
		}
	}

	return result, nil
}

func toGJSONPath(path string) string {
	p := strings.TrimPrefix(path, "/")
	return strings.ReplaceAll(p, "/", ".")
}

func modifyResponseCookie(headers []fetch.HeaderEntry, name, value string) []fetch.HeaderEntry {
	result := make([]fetch.HeaderEntry, 0, len(headers)+1)
	found := false
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Set-Cookie") {
			cookieName := name
			if idx := strings.Index(h.Value, "="); idx > 0 {
				cookieName = h.Value[:idx]
			}
			if strings.EqualFold(cookieName, name) {
				result = append(result, fetch.HeaderEntry{Name: "Set-Cookie", Value: name + "=" + value})
				found = true
				continue
			}
		}
		result = append(result, h)
	}
	if !found {
		result = append(result, fetch.HeaderEntry{Name: "Set-Cookie", Value: name + "=" + value})
	}
	return result
}

func removeResponseCookie(headers []fetch.HeaderEntry, name string) []fetch.HeaderEntry {
	result := make([]fetch.HeaderEntry, 0, len(headers))
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Set-Cookie") {
			cookieName := name
			if idx := strings.Index(h.Value, "="); idx > 0 {
				cookieName = h.Value[:idx]
			}
			if strings.EqualFold(cookieName, name) {
				continue
			}
		}
		result = append(result, h)
	}
	return result
}