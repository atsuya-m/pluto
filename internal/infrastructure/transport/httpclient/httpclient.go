package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ParseTarget(target string) (*url.URL, error) {
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse target: %w", err)
	}
	return u, nil
}

func NewHTTP1() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func NewHTTP2(u *url.URL) *http.Client {
	transport := &http.Transport{
		Proxy:       http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	protocols := new(http.Protocols)
	if u.Scheme == "https" {
		protocols.SetHTTP2(true)
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}
	transport.Protocols = protocols
	return &http.Client{Transport: transport}
}
