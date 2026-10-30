package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is an in-memory Domain Chief API that stores values the same way the real API does,
// so plan/apply cycles in tests surface any normalization the provider does not account for.
type fakeAPI struct {
	mu        sync.Mutex
	nextID    int
	records   map[string]map[string]any
	redirects map[string]map[string]any
	forwards  map[string]map[string]any
	// mailForwardingDisabled makes every mail forward request fail like a domain without mail forwarding.
	mailForwardingDisabled bool
}

const fakeDomain = "example.com"

func newFakeAPI(t *testing.T) (*fakeAPI, string) {
	t.Helper()

	api := &fakeAPI{
		records:   map[string]map[string]any{},
		redirects: map[string]map[string]any{},
		forwards:  map[string]map[string]any{},
	}

	server := httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(server.Close)

	return api, server.URL
}

func (a *fakeAPI) id() string {
	a.nextID++

	return fmt.Sprintf("01FAKE%04d", a.nextID)
}

func (a *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer test-token" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Unauthenticated."})

		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case len(parts) == 1 && parts[0] == "domains":
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{fakeDomainBody()}, "meta": map[string]any{"current_page": 1, "last_page": 1}})
	case len(parts) == 2 && parts[0] == "domains":
		if parts[1] != fakeDomain && parts[1] != "01DOMAIN" {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "Domain not found."})

			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": fakeDomainBody()})
	case len(parts) >= 4 && parts[0] == "domains" && parts[2] == "dns" && parts[3] == "records":
		a.collection(w, r, a.records, parts[4:], a.saveRecord)
	case len(parts) >= 4 && parts[0] == "domains" && parts[2] == "redirects" && parts[3] == "web":
		a.collection(w, r, a.redirects, parts[4:], a.saveRedirect)
	case len(parts) >= 4 && parts[0] == "domains" && parts[2] == "redirects" && parts[3] == "mail":
		if a.mailForwardingDisabled {
			writeJSON(w, http.StatusConflict, map[string]any{"message": "Mail forwarding is disabled.", "code": "mail_forwarding.disabled"})

			return
		}
		a.collection(w, r, a.forwards, parts[4:], a.saveForward)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found."})
	}
}

func (a *fakeAPI) collection(w http.ResponseWriter, r *http.Request, items map[string]map[string]any, rest []string, save func(existing, input map[string]any) map[string]any) {
	var input map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}

	if len(rest) == 0 {
		if r.Method == http.MethodGet {
			list := make([]any, 0, len(items))
			for _, id := range sortedKeys(items) {
				list = append(list, items[id])
			}

			writeJSON(w, http.StatusOK, map[string]any{"data": list, "meta": map[string]any{"current_page": 1, "last_page": 1}})

			return
		}

		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"message": "Method not allowed."})

			return
		}

		id := a.id()
		item := save(nil, input)
		item["id"] = id
		items[id] = item

		writeJSON(w, http.StatusCreated, map[string]any{"data": item})

		return
	}

	item, ok := items[rest[0]]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not found."})

		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodPut:
		updated := save(item, input)
		updated["id"] = item["id"]
		items[rest[0]] = updated
		writeJSON(w, http.StatusOK, map[string]any{"data": updated})
	case http.MethodDelete:
		delete(items, rest[0])
		w.WriteHeader(http.StatusNoContent)
	}
}

// saveRecord mirrors the record setters of the Domain Chief API.
func (a *fakeAPI) saveRecord(existing, input map[string]any) map[string]any {
	recordType, _ := input["type"].(string)

	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(stringValue(input["name"]))), ".")
	if name == "@" {
		name = ""
	}

	content := strings.TrimSpace(stringValue(input["content"]))
	if content == "@" {
		content = fakeDomain
	}

	switch {
	case recordType == "TXT":
		if !strings.HasPrefix(content, `"`) {
			content = `"` + content
		}
		if !strings.HasSuffix(content, `"`) {
			content += `"`
		}
	case recordType == "MX" && content == ".":
	case recordTypesWithHostnameContent[recordType]:
		content = strings.TrimSuffix(content, ".")
	}

	value := content
	if recordType == "TXT" {
		value = strings.Trim(content, `"`)
	}

	priority := 0.0
	if recordTypesWithPriority[recordType] {
		priority, _ = input["prio"].(float64)
	}

	var comment any
	if text, ok := input["comment"].(string); ok && text != "" {
		comment = text
	}

	metadata := map[string]string{}
	if current, ok := existing["metadata"].(map[string]string); ok {
		metadata = current
	}

	if changes, ok := input["metadata"].(map[string]any); ok {
		merged := map[string]string{}
		for key, current := range metadata {
			merged[key] = current
		}
		for key, change := range changes {
			if text, _ := change.(string); text == "" {
				delete(merged, key)
			} else {
				merged[key] = text
			}
		}
		metadata = merged

		// Domain Chief adds its own read-only keys next to the user metadata.
		if len(metadata) > 0 {
			metadata["domainchief:managed"] = "true"
		}
	}

	return map[string]any{
		"type":     recordType,
		"name":     name,
		"content":  content,
		"value":    value,
		"ttl":      input["ttl"],
		"prio":     priority,
		"comment":  comment,
		"metadata": metadata,
	}
}

// saveRedirect mirrors the web redirect API, the subdomain can only be set on create.
func (a *fakeAPI) saveRedirect(existing, input map[string]any) map[string]any {
	var subdomain any

	if existing != nil {
		subdomain = existing["subdomain"]
	} else if text := strings.ToLower(strings.TrimSpace(stringValue(input["subdomain"]))); text != "" {
		subdomain = text
	}

	includeWWW, _ := input["include_www"].(bool)
	if subdomain != nil {
		includeWWW = false
	}

	return map[string]any{
		"subdomain":     subdomain,
		"include_www":   includeWWW,
		"status_code":   input["status_code"],
		"target":        input["target"],
		"include_path":  input["include_path"],
		"include_query": input["include_query"],
	}
}

// saveForward mirrors the mail forwarding provider, which stores aliases lowercased.
func (a *fakeAPI) saveForward(_, input map[string]any) map[string]any {
	return map[string]any{
		"alias":  strings.ToLower(stringValue(input["alias"])),
		"target": input["target"],
	}
}

func fakeDomainBody() map[string]any {
	return map[string]any{
		"id":                       "01DOMAIN",
		"domain":                   fakeDomain,
		"unicode_domain":           fakeDomain,
		"status":                   "active",
		"is_locked":                true,
		"is_autorenew_enabled":     true,
		"is_whois_privacy_enabled": nil,
		"is_using_hosted_dns":      true,
		"is_dnssec_enabled":        true,
		"nameservers":              []any{map[string]any{"hostname": "ns.domainchief.eu", "ipv4": nil, "ipv6": nil}},
		"renews_at":                "2027-10-30T00:00:00.000000Z",
		"expires_at":               nil,
		"mail_forward_status":      "active",
		"metadata":                 map[string]string{"customer": "acme"},
	}
}

// seedRecord stores a record directly, like a record created outside Terraform.
func (a *fakeAPI) seedRecord(input, metadata map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()

	record := a.saveRecord(nil, input)
	for key, value := range metadata {
		record["metadata"].(map[string]string)[key] = value.(string) //nolint:forcetypeassert // test fixture
	}

	id := a.id()
	record["id"] = id
	a.records[id] = record
}

func sortedKeys(items map[string]map[string]any) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func stringValue(value any) string {
	text, _ := value.(string)

	return text
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
