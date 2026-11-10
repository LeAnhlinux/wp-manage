package security

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// managedInclude describes one family of per-site nginx snippets that the
// panel includes from a site's vhost (bot control, country access).
//
// Three rules, all learned the hard way:
//   - Files are keyed by SITE ID, not domain, so a domain change overwrites the
//     same files and two files can never define the same http-level variable.
//   - The include is a glob ("[.]conf"). nginx fails `nginx -t` on a plain
//     include of a missing file but silently skips a glob with no match, and
//     vhosts get copied verbatim to servers that do not have the snippet yet
//     (migration, backup restore).
//   - The include goes in the FIRST server block. With SSL the trailing block is
//     only the port-80 redirect, where rules would never see HTTPS traffic.
type managedInclude struct {
	dir string
	re  *regexp.Regexp
}

func newManagedInclude(dir string) managedInclude {
	return managedInclude{
		dir: dir,
		re:  regexp.MustCompile(`(?m)^[ \t]*include ` + regexp.QuoteMeta(dir) + `/site-(\d+)\[\.\]conf;[ \t]*\n`),
	}
}

var (
	botInclude = newManagedInclude("/etc/nginx/snippets/wpm-bots")
	geoInclude = newManagedInclude("/etc/nginx/snippets/wpm-geo")
)

func (m managedInclude) snippetPath(siteID uint64) string {
	return fmt.Sprintf("%s/site-%d.conf", m.dir, siteID)
}

func (m managedInclude) line(siteID uint64) string {
	return fmt.Sprintf("    include %s/site-%d[.]conf;\n", m.dir, siteID)
}

// set removes any include of this family from a vhost and, when want is true,
// adds the one for siteID at the end of the first server block.
func (m managedInclude) set(vhost string, siteID uint64, want bool) string {
	vhost = m.re.ReplaceAllString(vhost, "")
	if !want {
		return vhost
	}
	end := firstServerBlockEnd(vhost)
	if end < 0 {
		return vhost
	}
	return vhost[:end] + m.line(siteID) + vhost[end:]
}

// siteID returns the site id of this family's include in a vhost.
func (m managedInclude) siteID(vhost string) (uint64, bool) {
	match := m.re.FindStringSubmatch(vhost)
	if match == nil {
		return 0, false
	}
	id, err := strconv.ParseUint(match[1], 10, 64)
	return id, err == nil
}

func vhostPath(domain string) string {
	return fmt.Sprintf("/etc/nginx/sites-available/%s.conf", domain)
}

// fileBackup remembers a file's previous state so a failed apply can undo it.
type fileBackup struct {
	path    string
	data    []byte
	existed bool
}

func backupFile(path string) fileBackup {
	data, err := os.ReadFile(path)
	return fileBackup{path: path, data: data, existed: err == nil}
}

func (b fileBackup) restore() {
	if b.existed {
		os.WriteFile(b.path, b.data, 0644)
	} else {
		os.Remove(b.path)
	}
}

// writeOrRemoveFile writes content, or removes the file when content is empty.
func writeOrRemoveFile(path, content string) error {
	if content == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// nginxTestAndReload runs `nginx -t`; on failure it calls rollback and returns
// the test output. On success it reloads nginx.
func nginxTestAndReload(rollback func()) error {
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		rollback()
		return fmt.Errorf("nginx config test failed, changes rolled back: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// SiteIncludes records which managed snippets a domain's vhost includes. The
// vhost is the only place the agent can learn the site id for a domain, so
// capture this BEFORE any operation that rewrites or deletes the vhost.
type SiteIncludes struct {
	BotSiteID, GeoSiteID uint64
	HasBot, HasGeo       bool
}

// Any reports whether the vhost included anything.
func (s SiteIncludes) Any() bool { return s.HasBot || s.HasGeo }

// ManagedIncludesForDomain reads a domain's vhost and reports its managed includes.
func ManagedIncludesForDomain(domain string) SiteIncludes {
	var s SiteIncludes
	data, err := os.ReadFile(vhostPath(domain))
	if err != nil {
		return s
	}
	s.BotSiteID, s.HasBot = botInclude.siteID(string(data))
	s.GeoSiteID, s.HasGeo = geoInclude.siteID(string(data))
	return s
}

// RestoreManagedIncludes re-adds the includes after a vhost was regenerated
// from a template or overwritten from a backup (both drop them), then tests
// and reloads nginx. Snippets that no longer exist are skipped.
func RestoreManagedIncludes(domain string, s SiteIncludes) error {
	if !s.Any() {
		return nil
	}
	path := vhostPath(domain)
	backup := backupFile(path)
	if !backup.existed {
		return fmt.Errorf("nginx vhost not found for %s", domain)
	}
	vhost := string(backup.data)
	if s.HasBot {
		if _, err := os.Stat(botInclude.snippetPath(s.BotSiteID)); err == nil {
			vhost = botInclude.set(vhost, s.BotSiteID, true)
		}
	}
	if s.HasGeo {
		if _, err := os.Stat(geoInclude.snippetPath(s.GeoSiteID)); err == nil {
			vhost = geoInclude.set(vhost, s.GeoSiteID, true)
		}
	}
	if vhost == string(backup.data) {
		return nil
	}
	if err := os.WriteFile(path, []byte(vhost), 0644); err != nil {
		return err
	}
	return nginxTestAndReload(backup.restore)
}

// RemoveManagedFiles deletes a site's managed files after its vhost is gone.
func RemoveManagedFiles(s SiteIncludes) {
	if s.HasBot {
		os.Remove(botInclude.snippetPath(s.BotSiteID))
		os.Remove(botHTTPConfPath(s.BotSiteID))
	}
	if s.HasGeo {
		os.Remove(geoInclude.snippetPath(s.GeoSiteID))
		os.Remove(geoSiteConfPath(s.GeoSiteID))
	}
}
