package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Provider IP-range sources. Each provider publishes its own list; Meta has no
// JSON feed and documents the RADb whois query for its ASN instead.
var geoProviderJSONSources = map[string][]string{
	"google": {"https://www.gstatic.com/ipranges/goog.json"},
	"bing":   {"https://www.bing.com/toolbox/bingbot.json"},
	"openai": {"https://openai.com/gptbot.json", "https://openai.com/searchbot.json", "https://openai.com/chatgpt-user.json"},
	"apple":  {"https://search.developer.apple.com/applebot.json"},
}

var geoProviderTextSources = map[string][]string{
	"cloudflare": {"https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6"},
}

var geoProviderASNs = map[string]string{
	"meta": "AS32934",
}

// parsePrefixJSON reads the {"prefixes":[{"ipv4Prefix":..},{"ipv6Prefix":..}]}
// shape shared by Google, Bing, OpenAI and Apple.
func parsePrefixJSON(data []byte) []string {
	var doc struct {
		Prefixes []struct {
			IPv4 string `json:"ipv4Prefix"`
			IPv6 string `json:"ipv6Prefix"`
		} `json:"prefixes"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []string
	for _, p := range doc.Prefixes {
		if p.IPv4 != "" {
			out = append(out, p.IPv4)
		}
		if p.IPv6 != "" {
			out = append(out, p.IPv6)
		}
	}
	return out
}

// parseCIDRLines reads one CIDR per line, ignoring blanks and # comments.
func parseCIDRLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// parseRADbRoutes extracts route: / route6: objects from a whois response.
func parseRADbRoutes(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, prefix := range []string{"route6:", "route:"} {
			if strings.HasPrefix(line, prefix) {
				out = append(out, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
				break
			}
		}
	}
	return out
}

// cleanCIDRs keeps only well-formed networks, normalised and de-duplicated.
// Ranges broader than /8 (IPv4) or /16 (IPv6) are dropped: no provider owns
// that much, and one bad upstream entry such as 0.0.0.0/0 must never turn an
// "allow" or "deny" rule into one that covers the whole internet.
func cleanCIDRs(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		ones, bits := network.Mask.Size()
		if (bits == 32 && ones < 8) || (bits == 128 && ones < 16) {
			continue
		}
		s := network.String()
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func httpGetBody(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wp-manage-panel")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// whoisRADbOrigin lists the route objects registered for an ASN.
func whoisRADbOrigin(asn string) (string, error) {
	conn, err := net.DialTimeout("tcp", "whois.radb.net:43", 15*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(45 * time.Second))
	if _, err := fmt.Fprintf(conn, "-i origin %s\r\n", asn); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(conn, 8<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// fetchProviderCIDRs downloads and cleans the current ranges of one provider.
// Any failing source fails the whole provider, so a partial list never
// replaces a complete one.
func fetchProviderCIDRs(key string) ([]string, error) {
	var raw []string
	for _, url := range geoProviderJSONSources[key] {
		body, err := httpGetBody(url)
		if err != nil {
			return nil, err
		}
		prefixes := parsePrefixJSON(body)
		if len(prefixes) == 0 {
			return nil, fmt.Errorf("%s contained no prefixes", url)
		}
		raw = append(raw, prefixes...)
	}
	for _, url := range geoProviderTextSources[key] {
		body, err := httpGetBody(url)
		if err != nil {
			return nil, err
		}
		raw = append(raw, parseCIDRLines(string(body))...)
	}
	if asn, ok := geoProviderASNs[key]; ok {
		text, err := whoisRADbOrigin(asn)
		if err != nil {
			return nil, fmt.Errorf("whois %s: %w", asn, err)
		}
		raw = append(raw, parseRADbRoutes(text)...)
	}
	cidrs := cleanCIDRs(raw)
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("no valid IP ranges found for %s", key)
	}
	return cidrs, nil
}
