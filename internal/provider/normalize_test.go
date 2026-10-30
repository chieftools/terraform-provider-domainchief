package provider

import (
	"reflect"
	"testing"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

func TestNormalizeRecordName(t *testing.T) {
	cases := map[string]string{
		"":     "",
		"@":    "",
		"www":  "www",
		"WWW":  "www",
		"www.": "www",
		" mx ": "mx",
		"*.EU": "*.eu",
	}

	for input, expected := range cases {
		if got := normalizeRecordName(input); got != expected {
			t.Errorf("normalizeRecordName(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestNormalizeRecordContent(t *testing.T) {
	cases := []struct {
		name       string
		recordType string
		content    string
		domain     string
		expected   string
	}{
		{"txt unquoted", "TXT", "v=spf1 -all", "example.com", "v=spf1 -all"},
		{"txt quoted", "TXT", `"v=spf1 -all"`, "example.com", "v=spf1 -all"},
		{"txt multiple strings", "TXT", `"part one" "part two"`, "example.com", `part one" "part two`},
		{"cname trailing dot", "CNAME", "target.example.net.", "example.com", "target.example.net"},
		{"cname at sign", "CNAME", "@", "example.com", "example.com"},
		{"cname at sign with domain id", "CNAME", "@", "01hxyz", "@"},
		{"mx null", "MX", ".", "example.com", "."},
		{"mx trailing dot", "MX", "mail.example.com.", "example.com", "mail.example.com"},
		{"a record untouched", "A", " 192.0.2.1 ", "example.com", "192.0.2.1"},
		{"caa keeps quotes", "CAA", `0 issue "letsencrypt.org"`, "example.com", `0 issue "letsencrypt.org"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeRecordContent(tc.recordType, tc.content, tc.domain); got != tc.expected {
				t.Errorf("got %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestRecordContentFromAPI(t *testing.T) {
	cases := []struct {
		name     string
		record   client.DNSRecord
		expected string
	}{
		{"txt uses value", client.DNSRecord{Type: "TXT", Content: `"v=spf1 -all"`, Value: "v=spf1 -all"}, "v=spf1 -all"},
		{"other types use content", client.DNSRecord{Type: "A", Content: "192.0.2.1", Value: "192.0.2.1"}, "192.0.2.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := recordContentFromAPI(&tc.record); got != tc.expected {
				t.Errorf("got %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestUserMetadataDropsProtectedKeys(t *testing.T) {
	got := userMetadata(map[string]string{
		"owner":                      "team-a",
		"domainchief:sync_error":     "failed",
		"domainchief:acme_challenge": "true",
	})

	if !reflect.DeepEqual(got, map[string]string{"owner": "team-a"}) {
		t.Errorf("got %v", got)
	}
}

func TestMetadataChangesClearsRemovedKeys(t *testing.T) {
	got := metadataChanges(
		map[string]string{"owner": "team-a", "env": "prod"},
		map[string]string{"owner": "team-b"},
	)

	expected := map[string]string{"owner": "team-b", "env": ""}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, expected %v", got, expected)
	}
}
