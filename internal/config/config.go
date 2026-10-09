// Package config validates configuration without including secrets in errors.
package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL, Token, SPT, API, DB, HealthAddr, LogLevel string
	Topics                                             []string
	MinPriority, MaxPending                            int
	Stale, SendTimeout, RetryBase, RetryMax, Retention time.Duration
}

func value(k, d string) string {
	v, ok := os.LookupEnv(k)
	if !ok {
		return d
	}
	return strings.TrimSpace(v)
}
func secret(k string) (string, error) {
	v := value(k, "")
	p := value(k+"_FILE", "")
	if p != "" {
		if v != "" {
			return "", fmt.Errorf("set only %s or %s_FILE", k, k)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return "", fmt.Errorf("cannot read %s_FILE", k)
		}
		v = strings.TrimSpace(string(b))
	}
	if v == "" {
		return "", fmt.Errorf("%s is required", k)
	}
	if strings.ContainsAny(v, "\r\n") {
		return "", fmt.Errorf("%s contains invalid whitespace", k)
	}
	return v, nil
}
func validURL(k, v string) error {
	u, e := url.Parse(v)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must be an HTTP(S) URL without credentials, query or fragment", k)
	}
	return nil
}
func Load() (Config, error) {
	c := Config{BaseURL: strings.TrimRight(value("NTFY_BASE_URL", "http://ntfy"), "/"), API: value("WXPUSHER_API", "https://wxpusher.zjiecode.com/api/send/message/simple-push"), DB: value("STATE_DB", "/data/bridge.bolt"), LogLevel: strings.ToUpper(value("LOG_LEVEL", "INFO"))}
	var e error
	if c.Token, e = secret("NTFY_TOKEN"); e != nil {
		return c, e
	}
	if c.SPT, e = secret("WXPUSHER_SPT"); e != nil {
		return c, e
	}
	for k, v := range map[string]string{"NTFY_BASE_URL": c.BaseURL, "WXPUSHER_API": c.API} {
		if e = validURL(k, v); e != nil {
			return c, e
		}
	}
	seen := map[string]bool{}
	topicRE := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	for _, t := range strings.Split(value("NTFY_TOPICS", "notify"), ",") {
		t = strings.TrimSpace(t)
		if !topicRE.MatchString(t) || seen[t] {
			return c, fmt.Errorf("NTFY_TOPICS contains an empty, invalid or duplicate topic")
		}
		seen[t] = true
		c.Topics = append(c.Topics, t)
	}
	if c.DB == "" {
		return c, fmt.Errorf("STATE_DB is empty")
	}
	if c.LogLevel != "DEBUG" && c.LogLevel != "INFO" && c.LogLevel != "WARN" && c.LogLevel != "ERROR" {
		return c, fmt.Errorf("invalid LOG_LEVEL")
	}
	nums := map[string]*int{"FORWARD_MIN_PRIORITY": &c.MinPriority, "MAX_PENDING": &c.MaxPending}
	defs := map[string]string{"FORWARD_MIN_PRIORITY": "1", "MAX_PENDING": "10000"}
	for k, p := range nums {
		*p, e = strconv.Atoi(value(k, defs[k]))
		if e != nil || *p < 1 {
			return c, fmt.Errorf("%s must be a positive integer", k)
		}
	}
	if c.MinPriority > 5 {
		return c, fmt.Errorf("FORWARD_MIN_PRIORITY must be between 1 and 5")
	}
	port, e := strconv.Atoi(value("HEALTH_PORT", "8080"))
	if e != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("HEALTH_PORT must be between 1 and 65535")
	}
	c.HealthAddr = ":" + strconv.Itoa(port)
	ds := []struct {
		k, d string
		p    *time.Duration
		unit time.Duration
	}{{"NTFY_STALE_SECONDS", "180", &c.Stale, time.Second}, {"SEND_TIMEOUT", "15", &c.SendTimeout, time.Second}, {"RETRY_BASE_SECONDS", "2", &c.RetryBase, time.Second}, {"RETRY_MAX_SECONDS", "300", &c.RetryMax, time.Second}, {"DELIVERED_RETENTION_DAYS", "7", &c.Retention, 24 * time.Hour}}
	for _, x := range ds {
		n, er := strconv.ParseInt(value(x.k, x.d), 10, 64)
		if er != nil || n < 1 || n > int64((1<<63-1)/x.unit) {
			return c, fmt.Errorf("%s must be a positive bounded integer", x.k)
		}
		*x.p = time.Duration(n) * x.unit
	}
	if c.RetryMax < c.RetryBase {
		return c, fmt.Errorf("RETRY_MAX_SECONDS must be >= RETRY_BASE_SECONDS")
	}
	return c, nil
}
