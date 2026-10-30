package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, team string) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c := New(Config{BaseURL: server.URL, Token: "test-token", Team: team, UserAgent: "test"})
	c.retryBackoff = time.Millisecond

	return c
}

func TestRetriesWhileZoneIsBusy(t *testing.T) {
	var requests atomic.Int32

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 3 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Busy.","code":"dns.zone_busy"}`))

			return
		}

		_, _ = w.Write([]byte(`{"data":{"id":"rec","type":"A","name":"","content":"192.0.2.1","ttl":3600,"prio":0,"metadata":{}}}`))
	}, "")

	record, err := c.GetDNSRecord(context.Background(), "example.com", "rec")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if record.ID != "rec" || requests.Load() != 3 {
		t.Errorf("expected record after 3 requests, got %q after %d", record.ID, requests.Load())
	}
}

func TestDoesNotRetryPermanentConflicts(t *testing.T) {
	var requests atomic.Int32

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"The domain does not use hosted DNS.","code":"dns.not_hosted"}`))
	}, "")

	_, err := c.GetDNSRecord(context.Background(), "example.com", "rec")
	if err == nil || requests.Load() != 1 {
		t.Fatalf("expected one failed request, got %v after %d", err, requests.Load())
	}
}

func TestIsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not found."}`))
	}, "")

	_, err := c.GetDNSRecord(context.Background(), "example.com", "missing")
	if !IsNotFound(err) {
		t.Fatalf("expected a not found error, got %v", err)
	}
}

func TestSendsTeamHeaderOnlyWhenConfigured(t *testing.T) {
	for _, team := range []string{"", "team-id"} {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Chief-Team"); got != team {
				t.Errorf("expected X-Chief-Team %q, got %q", team, got)
			}

			_, _ = w.Write([]byte(`{"data":{"id":"domain"}}`))
		}, team)

		if _, err := c.GetDomain(context.Background(), "example.com"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}
}

func TestListFollowsEveryPage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")

		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("expected per_page=100, got %q", r.URL.Query().Get("per_page"))
		}

		_, _ = fmt.Fprintf(w, `{"data":[{"id":"domain-%s"}],"meta":{"current_page":%s,"last_page":2}}`, page, page)
	}, "")

	domains, err := c.ListDomains(context.Background(), "", "active")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(domains) != 2 || domains[0].ID != "domain-1" || domains[1].ID != "domain-2" {
		t.Errorf("expected domains from both pages, got %+v", domains)
	}
}

func TestValidationErrorsAreIncludedInMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"The name is invalid.","errors":{"name":["The name is invalid."]}}`))
	}, "")

	_, err := c.CreateDNSRecord(context.Background(), "example.com", DNSRecordInput{Type: "A"})

	expected := "domain chief API error (422): The name is invalid. [name: The name is invalid.]"
	if err == nil || err.Error() != expected {
		t.Fatalf("expected %q, got %v", expected, err)
	}
}
