package service

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

const (
	captureHeaderPartBudget = 16 << 10
	captureHeaderFieldLimit = 64
	captureHeaderValueLimit = 128
	captureHeaderNameLimit  = 128
	captureHeaderRedaction  = "[REDACTED]"
)

// captureHeaders records the headers visible at the application HTTP boundary.
// The body budget stays separate, and unrecognized values are masked before
// any data is retained because providers can authenticate with custom headers.
func (p *CapturePart) captureHeaders(headers http.Header, host string, length int64, transferEncoding []string) {
	remaining, valuesLeft := captureHeaderPartBudget, captureHeaderValueLimit
	fieldsLeft := captureHeaderFieldLimit
	add := func(name string, values []string, preserve bool) {
		if len(values) == 0 {
			return
		}
		if len(name) > captureHeaderNameLimit || !httpguts.ValidHeaderFieldName(name) {
			p.HeadersTruncated = true
			return
		}
		name = http.CanonicalHeaderKey(name)
		if len(p.Headers) >= captureHeaderFieldLimit && p.Headers[name] == nil {
			p.HeadersTruncated = true
			return
		}
		if !preserve {
			values = []string{captureHeaderRedaction}
			p.HeadersRedacted = true
		}
		for _, value := range values {
			// Include JSON field and value separators in the retained budget.
			if valuesLeft == 0 || len(name)+len(value)+8 > remaining {
				p.HeadersTruncated = true
				break
			}
			if preserve {
				clean := captureHeaderValue(name, value)
				p.HeadersRedacted = p.HeadersRedacted || clean != value
				value = clean
			}
			used := len(name) + len(value) + 8
			if used > remaining {
				p.HeadersTruncated = true
				break
			}
			if p.Headers == nil {
				p.Headers = make(http.Header)
			}
			p.Headers[name] = append(p.Headers[name], strings.Clone(value))
			remaining -= used
			valuesLeft--
		}
	}
	if host != "" && len(headers.Values("Host")) == 0 {
		add("Host", []string{host}, true)
	}
	if length > 0 && len(headers.Values("Content-Length")) == 0 {
		add("Content-Length", []string{strconv.FormatInt(length, 10)}, true)
	}
	if len(transferEncoding) > 0 && len(headers.Values("Transfer-Encoding")) == 0 {
		add("Transfer-Encoding", transferEncoding, true)
	}
	for name, values := range headers {
		if remaining < 9 || valuesLeft == 0 || fieldsLeft == 0 {
			p.HeadersTruncated = true
			break
		}
		fieldsLeft--
		if len(name) > captureHeaderNameLimit {
			p.HeadersTruncated = true
			continue
		}
		switch strings.ToLower(name) {
		case "accept", "accept-charset", "accept-encoding", "accept-language", "age", "allow",
			"cache-control", "connection", "content-disposition", "content-encoding",
			"content-language", "content-length", "content-range", "content-type",
			"date", "etag", "expect", "expires", "host", "if-match", "if-modified-since",
			"if-none-match", "if-range", "if-unmodified-since", "last-modified", "location",
			"origin", "pragma", "range", "referer", "retry-after", "server", "te", "trailer",
			"transfer-encoding", "upgrade", "user-agent", "vary",
			"anthropic-version", "openai-processing-ms", "openai-version",
			"request-id", "traceparent", "x-amzn-requestid", "x-accel-buffering",
			"x-oneapi-request-id", "x-request-id", "x-trace-id",
			"msh-usage-cache-write-tokens-1h", "msh-usage-cache-write-tokens-5m",
			"x-ratelimit-limit-requests", "x-ratelimit-limit-tokens",
			"x-ratelimit-remaining-requests", "x-ratelimit-remaining-tokens",
			"x-ratelimit-reset-requests", "x-ratelimit-reset-tokens":
			add(name, values, true)
		default:
			add(name, values, false)
		}
	}
}

func captureHeaderValue(name, value string) string {
	if !utf8.ValidString(value) || !httpguts.ValidHeaderFieldValue(value) {
		return captureHeaderRedaction
	}
	if scheme, _, ok := strings.Cut(value, " "); ok &&
		(strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "Basic") || strings.EqualFold(scheme, "Digest") || strings.EqualFold(scheme, "AWS4-HMAC-SHA256")) {
		return captureHeaderRedaction
	}
	switch name {
	case "Location", "Referer", "Origin":
		u, err := url.Parse(value)
		if err != nil || u.Opaque != "" {
			return captureHeaderRedaction
		}
		if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			u.User, u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = nil, "", "", "", false
			return u.String()
		}
	case "Host":
		if strings.ContainsAny(value, "@?#") {
			return captureHeaderRedaction
		}
	}
	return value
}
