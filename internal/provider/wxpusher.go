// Package provider implements the WxPusher SPT delivery adapter.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Syt100/notify-relay/internal/message"
)

type WxPusher struct {
	api, spt string
	client   *http.Client
}
type DeliveryError struct {
	Reason string
	After  time.Duration
}

func (e *DeliveryError) Error() string { return e.Reason }
func New(api, spt string, timeout time.Duration) *WxPusher {
	return &WxPusher{api: api, spt: spt, client: &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout, MaxIdleConns: 2, MaxIdleConnsPerHost: 1, IdleConnTimeout: 90 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func retryAfter(v string) time.Duration {
	if n, e := strconv.Atoi(v); e == nil && n > 0 {
		if n > 3600 {
			n = 3600
		}
		return time.Duration(n) * time.Second
	}
	if ts, e := http.ParseTime(v); e == nil {
		d := time.Until(ts)
		if d > time.Hour {
			return time.Hour
		}
		if d > 0 {
			return d
		}
	}
	return 0
}
func (p *WxPusher) Send(ctx context.Context, m message.Message) error {
	raw, e := json.Marshal(message.Payload(m, p.spt))
	if e != nil {
		return &DeliveryError{Reason: "payload encoding failed"}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.api, bytes.NewReader(raw))
	if e != nil {
		return &DeliveryError{Reason: "invalid provider URL"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "notify-relay")
	resp, e := p.client.Do(req)
	if e != nil {
		return &DeliveryError{Reason: "provider connection failed or timed out"}
	}
	defer resp.Body.Close()
	after := retryAfter(resp.Header.Get("Retry-After"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &DeliveryError{Reason: fmt.Sprintf("provider HTTP %d", resp.StatusCode), After: after}
	}
	raw, e = io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(raw) > 65536 {
		return &DeliveryError{Reason: "provider response unreadable or too large"}
	}
	var result struct {
		Code    int   `json:"code"`
		Success *bool `json:"success"`
	}
	if e = json.Unmarshal(raw, &result); e != nil {
		return &DeliveryError{Reason: "provider response is not JSON"}
	}
	if result.Code != 1000 || (result.Success != nil && !*result.Success) {
		return &DeliveryError{Reason: fmt.Sprintf("provider business code %d", result.Code), After: after}
	}
	return nil
}
