package client

import (
	"context"
	"net/http"
	"net/url"
)

// DNSRecord is a hosted DNS record.
type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	// Value is the content without zone file quoting, it only differs from Content for TXT records.
	Value    string            `json:"value"`
	TTL      int64             `json:"ttl"`
	Priority int64             `json:"prio"`
	Comment  *string           `json:"comment"`
	Metadata map[string]string `json:"metadata"`
}

// DNSRecordInput is the body for creating or updating a DNS record.
type DNSRecordInput struct {
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Content  string            `json:"content"`
	TTL      int64             `json:"ttl"`
	Priority *int64            `json:"prio,omitempty"`
	Comment  *string           `json:"comment"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func recordsPath(domain string) string {
	return "/domains/" + escape(domain) + "/dns/records"
}

// GetDNSRecord retrieves a single DNS record.
func (c *Client) GetDNSRecord(ctx context.Context, domain, id string) (*DNSRecord, error) {
	return get[DNSRecord](ctx, c, recordsPath(domain)+"/"+escape(id), nil)
}

// ListDNSRecords retrieves every DNS record of a domain.
func (c *Client) ListDNSRecords(ctx context.Context, domain string) ([]DNSRecord, error) {
	return list[DNSRecord](ctx, c, recordsPath(domain), nil)
}

// CreateDNSRecord creates a DNS record.
func (c *Client) CreateDNSRecord(ctx context.Context, domain string, input DNSRecordInput) (*DNSRecord, error) {
	return send[DNSRecord](ctx, c, http.MethodPost, recordsPath(domain), input)
}

// UpdateDNSRecord replaces a DNS record.
func (c *Client) UpdateDNSRecord(ctx context.Context, domain, id string, input DNSRecordInput) (*DNSRecord, error) {
	return send[DNSRecord](ctx, c, http.MethodPut, recordsPath(domain)+"/"+escape(id), input)
}

// DeleteDNSRecord deletes a DNS record.
func (c *Client) DeleteDNSRecord(ctx context.Context, domain, id string) error {
	return c.do(ctx, http.MethodDelete, recordsPath(domain)+"/"+escape(id), nil, nil, nil)
}

// WebRedirect is a web redirect.
type WebRedirect struct {
	ID           string  `json:"id"`
	Subdomain    *string `json:"subdomain"`
	IncludeWWW   bool    `json:"include_www"`
	StatusCode   int64   `json:"status_code"`
	Target       string  `json:"target"`
	IncludePath  bool    `json:"include_path"`
	IncludeQuery bool    `json:"include_query"`
}

// WebRedirectInput is the body for creating or updating a web redirect, the subdomain is ignored on update.
type WebRedirectInput struct {
	Subdomain    *string `json:"subdomain,omitempty"`
	IncludeWWW   bool    `json:"include_www"`
	StatusCode   int64   `json:"status_code"`
	Target       string  `json:"target"`
	IncludePath  bool    `json:"include_path"`
	IncludeQuery bool    `json:"include_query"`
}

func webRedirectsPath(domain string) string {
	return "/domains/" + escape(domain) + "/redirects/web"
}

// GetWebRedirect retrieves a single web redirect.
func (c *Client) GetWebRedirect(ctx context.Context, domain, id string) (*WebRedirect, error) {
	return get[WebRedirect](ctx, c, webRedirectsPath(domain)+"/"+escape(id), nil)
}

// CreateWebRedirect creates a web redirect.
func (c *Client) CreateWebRedirect(ctx context.Context, domain string, input WebRedirectInput) (*WebRedirect, error) {
	return send[WebRedirect](ctx, c, http.MethodPost, webRedirectsPath(domain), input)
}

// UpdateWebRedirect updates a web redirect.
func (c *Client) UpdateWebRedirect(ctx context.Context, domain, id string, input WebRedirectInput) (*WebRedirect, error) {
	input.Subdomain = nil

	return send[WebRedirect](ctx, c, http.MethodPut, webRedirectsPath(domain)+"/"+escape(id), input)
}

// DeleteWebRedirect deletes a web redirect.
func (c *Client) DeleteWebRedirect(ctx context.Context, domain, id string) error {
	return c.do(ctx, http.MethodDelete, webRedirectsPath(domain)+"/"+escape(id), nil, nil, nil)
}

// MailForward is a mail forward.
type MailForward struct {
	ID     string `json:"id"`
	Alias  string `json:"alias"`
	Target string `json:"target"`
}

// MailForwardInput is the body for creating or updating a mail forward.
type MailForwardInput struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
}

func mailForwardsPath(domain string) string {
	return "/domains/" + escape(domain) + "/redirects/mail"
}

// GetMailForward retrieves a single mail forward.
func (c *Client) GetMailForward(ctx context.Context, domain, id string) (*MailForward, error) {
	return get[MailForward](ctx, c, mailForwardsPath(domain)+"/"+escape(id), nil)
}

// CreateMailForward creates a mail forward.
func (c *Client) CreateMailForward(ctx context.Context, domain string, input MailForwardInput) (*MailForward, error) {
	return send[MailForward](ctx, c, http.MethodPost, mailForwardsPath(domain), input)
}

// UpdateMailForward updates a mail forward.
func (c *Client) UpdateMailForward(ctx context.Context, domain, id string, input MailForwardInput) (*MailForward, error) {
	return send[MailForward](ctx, c, http.MethodPut, mailForwardsPath(domain)+"/"+escape(id), input)
}

// DeleteMailForward deletes a mail forward.
func (c *Client) DeleteMailForward(ctx context.Context, domain, id string) error {
	return c.do(ctx, http.MethodDelete, mailForwardsPath(domain)+"/"+escape(id), nil, nil, nil)
}

// Nameserver is a nameserver of a domain.
type Nameserver struct {
	Hostname string  `json:"hostname"`
	IPv4     *string `json:"ipv4"`
	IPv6     *string `json:"ipv6"`
}

// Domain is a domain in the team.
type Domain struct {
	ID                    string            `json:"id"`
	Domain                string            `json:"domain"`
	UnicodeDomain         string            `json:"unicode_domain"`
	Status                string            `json:"status"`
	IsLocked              *bool             `json:"is_locked"`
	IsAutorenewEnabled    bool              `json:"is_autorenew_enabled"`
	IsWhoisPrivacyEnabled *bool             `json:"is_whois_privacy_enabled"`
	IsUsingHostedDNS      bool              `json:"is_using_hosted_dns"`
	IsDNSSECEnabled       *bool             `json:"is_dnssec_enabled"`
	Nameservers           []Nameserver      `json:"nameservers"`
	RenewsAt              *string           `json:"renews_at"`
	ExpiresAt             *string           `json:"expires_at"`
	MailForwardStatus     string            `json:"mail_forward_status"`
	Metadata              map[string]string `json:"metadata"`
}

// GetDomain retrieves a domain by name or ID.
func (c *Client) GetDomain(ctx context.Context, domain string) (*Domain, error) {
	return get[Domain](ctx, c, "/domains/"+escape(domain), nil)
}

// ListDomains retrieves every domain, optionally filtered by a search query and status.
func (c *Client) ListDomains(ctx context.Context, query, status string) ([]Domain, error) {
	values := url.Values{}

	if query != "" {
		values.Set("query", query)
	}

	if status != "" {
		values.Set("status", status)
	}

	return list[Domain](ctx, c, "/domains", values)
}
