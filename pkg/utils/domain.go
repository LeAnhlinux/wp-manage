package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
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

// maxLinuxUserLen is the longest user name useradd accepts.
const maxLinuxUserLen = 32

// SiteLinuxUser returns the Linux user that owns a site's files and runs its
// PHP-FPM pool: "wp_" + the domain with dots and hyphens turned into
// underscores. Names that would exceed the 32-character limit of useradd are
// shortened to a prefix plus a short hash of the domain, so they stay unique
// and stable. Every package must derive the name through this function.
func SiteLinuxUser(domain string) string {
	name := "wp_" + strings.NewReplacer(".", "_", "-", "_").Replace(domain)
	if len(name) <= maxLinuxUserLen {
		return name
	}
	sum := sha256.Sum256([]byte(domain))
	suffix := "_" + hex.EncodeToString(sum[:])[:6]
	return name[:maxLinuxUserLen-len(suffix)] + suffix
}
