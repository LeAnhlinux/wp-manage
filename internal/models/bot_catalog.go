package models

import "regexp"

// Bot control actions.
const (
	BotActionAllow     = "allow"
	BotActionDeny      = "deny"
	BotActionRateLimit = "rate_limit"
)

// BotRateTiers are the only allowed rate limits (requests per minute). Each
// distinct tier costs one nginx limit_req zone per site, so the set is fixed.
var BotRateTiers = []int{10, 30, 60, 120}

// BotCatalogEntry is a well-known bot the panel offers as a preset.
type BotCatalogEntry struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Category string `json:"category"` // search | ai | seo | scraper
	Pattern  string `json:"pattern"`  // literal User-Agent substring (case-insensitive)
}

// BotCatalog is the preset list. Pattern must satisfy ValidBotPattern.
var BotCatalog = []BotCatalogEntry{
	// Search engines
	{"googlebot", "Googlebot", "search", "Googlebot"},
	{"bingbot", "Bingbot", "search", "bingbot"},
	{"yandexbot", "YandexBot", "search", "YandexBot"},
	{"baiduspider", "Baiduspider", "search", "Baiduspider"},
	{"duckduckbot", "DuckDuckBot", "search", "DuckDuckBot"},
	{"applebot", "Applebot", "search", "Applebot"},
	{"coccocbot", "CocCocBot", "search", "coccocbot"},
	// AI crawlers
	{"gptbot", "GPTBot", "ai", "GPTBot"},
	{"chatgpt-user", "ChatGPT-User", "ai", "ChatGPT-User"},
	{"oai-searchbot", "OAI-SearchBot", "ai", "OAI-SearchBot"},
	{"claudebot", "ClaudeBot", "ai", "ClaudeBot"},
	{"anthropic-ai", "anthropic-ai", "ai", "anthropic-ai"},
	{"perplexitybot", "PerplexityBot", "ai", "PerplexityBot"},
	{"google-extended", "Google-Extended", "ai", "Google-Extended"},
	{"ccbot", "CCBot", "ai", "CCBot"},
	{"bytespider", "Bytespider", "ai", "Bytespider"},
	{"amazonbot", "Amazonbot", "ai", "Amazonbot"},
	{"meta-externalagent", "meta-externalagent", "ai", "meta-externalagent"},
	// SEO tools
	{"ahrefsbot", "AhrefsBot", "seo", "AhrefsBot"},
	{"semrushbot", "SemrushBot", "seo", "SemrushBot"},
	{"mj12bot", "MJ12bot", "seo", "MJ12bot"},
	{"dotbot", "DotBot", "seo", "DotBot"},
	{"dataforseobot", "DataForSeoBot", "seo", "DataForSeoBot"},
	{"blexbot", "BLEXBot", "seo", "BLEXBot"},
	{"petalbot", "PetalBot", "seo", "PetalBot"},
	{"seznambot", "SeznamBot", "seo", "SeznamBot"},
	// Scrapers & HTTP clients
	{"python-requests", "python-requests", "scraper", "python-requests"},
	{"scrapy", "Scrapy", "scraper", "Scrapy"},
	{"curl", "curl", "scraper", "curl/"},
	{"wget", "Wget", "scraper", "Wget/"},
	{"go-http-client", "Go-http-client", "scraper", "Go-http-client"},
	{"httrack", "HTTrack", "scraper", "HTTrack"},
	{"libwww-perl", "libwww-perl", "scraper", "libwww-perl"},
}

// FindBot looks a preset up by key.
func FindBot(key string) (BotCatalogEntry, bool) {
	for _, b := range BotCatalog {
		if b.Key == key {
			return b, true
		}
	}
	return BotCatalogEntry{}, false
}

var botPatternRegex = regexp.MustCompile(`^[A-Za-z0-9 ._/-]{2,100}$`)

// ValidBotPattern reports whether p is safe to embed in an nginx map regex.
// Checked by the API on save AND by the agent before writing config, because
// the pattern ends up inside a quoted nginx string.
func ValidBotPattern(p string) bool {
	return botPatternRegex.MatchString(p)
}

// ValidBotRate reports whether n is one of BotRateTiers.
func ValidBotRate(n int) bool {
	for _, t := range BotRateTiers {
		if t == n {
			return true
		}
	}
	return false
}
