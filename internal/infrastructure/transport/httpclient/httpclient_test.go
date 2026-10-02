package httpclient_test

import (
	"net/url"
	"testing"

	"github.com/atsuya-m/pluto/internal/infrastructure/transport/httpclient"
)

func TestClientsHaveNoOverallTimeout(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	if got := httpclient.NewHTTP1().Timeout; got != 0 {
		t.Errorf("NewHTTP1().Timeout = %v, want 0 so that server streaming is not cut off", got)
	}
	if got := httpclient.NewHTTP2(u).Timeout; got != 0 {
		t.Errorf("NewHTTP2().Timeout = %v, want 0", got)
	}
}
