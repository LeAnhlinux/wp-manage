package utils

import (
	"fmt"
	"regexp"
)

// domainRegex matches valid hostnames per RFC 1123:
// - Labels of 1–63 chars containing [a-z0-9] and hyphens (not at start/end)
// - Total length ≤ 253 chars
// - At least one dot (so bare hostnames like "localhost" are rejected)
var domainRegex = regexp.MustCompile(
	`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`,
)

// ValidateDomain returns an error if domain is not a valid hostname.
// Accepts lowercase letters, digits, hyphens and dots only.
// Rejects: empty, > 253 chars, path traversal components, uppercase letters.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if len(domain) > 253 {
		return fmt.Errorf("domain exceeds maximum length of 253 characters")
	}
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain %q: must be a valid hostname (lowercase letters, digits, hyphens, dots only)", domain)
	}
	return nil
}
