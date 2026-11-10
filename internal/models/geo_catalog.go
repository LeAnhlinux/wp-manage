package models

import "strings"

// Country-access modes and provider actions.
const (
	GeoModeOff       = "off"
	GeoModeBlacklist = "blacklist" // block the selected countries
	GeoModeWhitelist = "whitelist" // allow only the selected countries

	GeoActionAllow = "allow"
	GeoActionDeny  = "deny"
)

// GeoProvider is a large network operator whose published IP ranges can be
// allowed or denied independently of the country rules.
type GeoProvider struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GeoProviders is the provider catalog. Order is the priority used when two
// providers publish the same range (first one wins).
var GeoProviders = []GeoProvider{
	{"cloudflare", "Cloudflare", "Cloudflare proxy edge ranges"},
	{"google", "Google", "All Google ranges, including Googlebot and Google Cloud services"},
	{"meta", "Meta / Facebook", "Facebook, Instagram and WhatsApp crawlers and link previews (AS32934)"},
	{"bing", "Microsoft Bing", "Bingbot crawler ranges"},
	{"openai", "OpenAI", "GPTBot, OAI-SearchBot and ChatGPT-User"},
	{"apple", "Apple", "Applebot crawler ranges"},
}

// FindGeoProvider looks a provider up by key.
func FindGeoProvider(key string) (GeoProvider, bool) {
	for _, p := range GeoProviders {
		if p.Key == key {
			return p, true
		}
	}
	return GeoProvider{}, false
}

// geoCountryCodes is ISO 3166-1 alpha-2 plus XK (Kosovo), which GeoIP
// databases use. Display names come from the browser (Intl.DisplayNames).
const geoCountryCodes = "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ " +
	"CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR " +
	"GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP " +
	"KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ " +
	"NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW " +
	"SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ " +
	"UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS XK YE YT ZA ZM ZW"

// GeoCountryCodes lists every selectable country code, sorted.
var GeoCountryCodes = strings.Fields(geoCountryCodes)

var geoCountrySet = func() map[string]bool {
	m := make(map[string]bool, len(GeoCountryCodes))
	for _, c := range GeoCountryCodes {
		m[c] = true
	}
	return m
}()

// ValidCountryCode reports whether c is a known upper-case country code.
// Checked by the API and again by the agent: the code is written into nginx config.
func ValidCountryCode(c string) bool {
	return geoCountrySet[c]
}

// ValidGeoMode reports whether m is a known mode.
func ValidGeoMode(m string) bool {
	return m == GeoModeOff || m == GeoModeBlacklist || m == GeoModeWhitelist
}
