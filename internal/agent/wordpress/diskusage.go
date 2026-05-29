package wordpress

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// dbNameRe guards the DB name before it goes into a raw SQL string — only the
// charset our own DB-naming uses. Prevents SQL injection via a tampered task.
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// ScanDiskUsage computes file-tree + database size for each site in one pass.
// File size: `du -sb /var/www/{domain}` (bytes). DB size: information_schema
// sum (root socket auth, same pattern as database/users.go). Errors on a single
// site degrade to 0 for that metric rather than failing the whole scan.
func ScanDiskUsage(sites []models.DiskScanSite) []models.DiskScanSiteResult {
	results := make([]models.DiskScanSiteResult, 0, len(sites))
	for _, st := range sites {
		r := models.DiskScanSiteResult{Domain: st.Domain}

		// Files — du -sb returns "bytes\tpath"
		if out, err := exec.Command("du", "-sb", fmt.Sprintf("/var/www/%s", st.Domain)).Output(); err == nil {
			fields := strings.Fields(string(out))
			if len(fields) > 0 {
				r.FilesBytes, _ = strconv.ParseInt(fields[0], 10, 64)
			}
		}

		// Database — sum data_length+index_length (bytes)
		if st.DBName != "" && dbNameRe.MatchString(st.DBName) {
			q := fmt.Sprintf(
				`SELECT IFNULL(SUM(data_length+index_length),0) FROM information_schema.TABLES WHERE TABLE_SCHEMA='%s'`,
				st.DBName)
			if out, err := exec.Command("mysql", "-N", "-e", q).Output(); err == nil {
				r.DBBytes, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			}
		}

		results = append(results, r)
	}
	return results
}
