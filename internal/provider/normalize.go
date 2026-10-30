package provider

import (
	"strings"

	"github.com/chieftools/terraform-provider-domainchief/internal/client"
)

// protectedMetadataPrefix marks metadata keys managed by Domain Chief itself, they are read-only and never part of the configuration.
const protectedMetadataPrefix = "domainchief:"

// recordTypesWithHostnameContent stores their content without a trailing dot.
var recordTypesWithHostnameContent = map[string]bool{
	"CNAME": true,
	"MX":    true,
	"NS":    true,
	"ALIAS": true,
	"PTR":   true,
}

// recordTypesWithPriority are the only types that use a priority, other types always have a priority of 0.
var recordTypesWithPriority = map[string]bool{
	"MX":  true,
	"SRV": true,
}

// allowedTTLs are the TTL values hosted DNS supports, the API rounds other values to the closest one.
var allowedTTLs = []int64{60, 300, 900, 1800, 3600, 21600, 43200, 86400, 172800, 259200, 604800, 1209600, 2592000}

// normalizeRecordName mirrors how Domain Chief stores record names: lowercased, without a trailing dot and with `@` as the empty apex.
func normalizeRecordName(name string) string {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")

	if name == "@" {
		return ""
	}

	return name
}

// normalizeRecordContent mirrors how Domain Chief stores record content, using the unquoted form for TXT records.
func normalizeRecordContent(recordType, content, domain string) string {
	content = strings.TrimSpace(content)

	if content == "@" && strings.Contains(domain, ".") {
		content = strings.ToLower(domain)
	}

	switch {
	case recordType == "TXT":
		return strings.Trim(content, `"`)
	case recordType == "MX" && content == ".":
		return content
	case recordTypesWithHostnameContent[recordType]:
		return strings.TrimSuffix(content, ".")
	default:
		return content
	}
}

// recordContentFromAPI returns the record content in the form the provider keeps in state, TXT records are kept unquoted.
func recordContentFromAPI(record *client.DNSRecord) string {
	if record.Type == "TXT" {
		return record.Value
	}

	return record.Content
}

// userMetadata drops the read-only metadata keys managed by Domain Chief.
func userMetadata(metadata map[string]string) map[string]string {
	result := make(map[string]string, len(metadata))

	for key, value := range metadata {
		if !strings.HasPrefix(key, protectedMetadataPrefix) {
			result[key] = value
		}
	}

	return result
}

// metadataChanges builds the metadata to send on update: the API merges metadata, so removed keys are sent with an empty value.
func metadataChanges(previous, desired map[string]string) map[string]string {
	changes := make(map[string]string, len(desired))

	for key, value := range desired {
		changes[key] = value
	}

	for key := range previous {
		if _, ok := desired[key]; !ok {
			changes[key] = ""
		}
	}

	return changes
}
