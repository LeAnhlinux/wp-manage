package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber-team/wp-manage/internal/models"
)

// OptimizeDatabase optimizes tables, cleans revisions and transients
func OptimizeDatabase(task models.AgentDBOptimizeTask) models.AgentDBOptimizeResult {
	result := models.AgentDBOptimizeResult{
		SiteID: task.SiteID,
		Status: "completed",
	}

	dsn := fmt.Sprintf("%s:%s@tcp(127.0.0.1:3306)/%s?parseTime=true", task.DBUser, task.DBPass, task.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to connect: %v", err)
		return result
	}
	defer db.Close()

	// Detect WordPress table prefix from the actual DB
	// (sites can use non-default prefix like motphim_, my_, etc.)
	prefix := detectTablePrefix(db, task.DBName)
	slog.Info("Detected WP table prefix", "db", task.DBName, "prefix", prefix)

	// 1. Optimize all tables
	result.TablesOptimized = optimizeTables(db, task.DBName)

	// 2. Calculate space saved (approximate from fragmented space)
	result.SpaceSaved = getFragmentedSpace(db, task.DBName)

	// 3. Clean post revisions (keep last 5 per post)
	result.RevisionsClean = cleanRevisions(db, prefix)

	// 4. Clear expired transients
	result.TransientsClear = clearTransients(db, prefix)

	slog.Info("DB optimization completed",
		"db", task.DBName,
		"prefix", prefix,
		"tables", result.TablesOptimized,
		"space_saved", result.SpaceSaved,
		"revisions_cleaned", result.RevisionsClean,
		"transients_cleared", result.TransientsClear,
	)

	return result
}

// detectTablePrefix finds the WordPress table prefix by looking for the options table
// e.g. wp_options → "wp_", motphim_options → "motphim_"
func detectTablePrefix(db *sql.DB, dbName string) string {
	rows, err := db.Query(
		"SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME LIKE '%options' ORDER BY TABLE_NAME LIMIT 5",
		dbName,
	)
	if err != nil {
		return "wp_"
	}
	defer rows.Close()
	for rows.Next() {
		var tableName string
		rows.Scan(&tableName)
		// Verify it actually has siteurl (confirms it's the WP options table)
		var cnt int
		db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE option_name='siteurl' LIMIT 1", tableName)).Scan(&cnt)
		if cnt > 0 {
			return strings.TrimSuffix(tableName, "options")
		}
	}
	return "wp_"
}

func optimizeTables(db *sql.DB, dbName string) int {
	rows, err := db.Query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND ENGINE = 'InnoDB'", dbName)
	if err != nil {
		slog.Error("Failed to list tables", "error", err)
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var tableName string
		rows.Scan(&tableName)

		_, err := db.Exec(fmt.Sprintf("OPTIMIZE TABLE `%s`", tableName))
		if err != nil {
			slog.Warn("Failed to optimize table", "table", tableName, "error", err)
			continue
		}
		count++
	}
	return count
}

func getFragmentedSpace(db *sql.DB, dbName string) int64 {
	var total sql.NullInt64
	err := db.QueryRow(`
		SELECT SUM(DATA_FREE) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND ENGINE = 'InnoDB'
	`, dbName).Scan(&total)
	if err != nil || !total.Valid {
		return 0
	}
	return total.Int64
}

func cleanRevisions(db *sql.DB, prefix string) int {
	postsTable := fmt.Sprintf("`%sposts`", prefix)

	// Delete all revisions except the last 5 per parent post
	res, err := db.Exec(fmt.Sprintf(`
		DELETE FROM %s WHERE post_type = 'revision'
		AND ID NOT IN (
			SELECT sub.ID FROM (
				SELECT r.ID,
					ROW_NUMBER() OVER (PARTITION BY r.post_parent ORDER BY r.post_date DESC) as rn
				FROM %s r WHERE r.post_type = 'revision'
			) sub WHERE sub.rn <= 5
		)
	`, postsTable, postsTable))
	if err != nil {
		// Fallback: delete revisions older than 30 days
		res, err = db.Exec(fmt.Sprintf(`
			DELETE FROM %s
			WHERE post_type = 'revision'
			AND post_date < DATE_SUB(NOW(), INTERVAL 30 DAY)
		`, postsTable))
		if err != nil {
			slog.Warn("Failed to clean revisions", "table", postsTable, "error", err)
			return 0
		}
	}

	affected, _ := res.RowsAffected()
	return int(affected)
}

func clearTransients(db *sql.DB, prefix string) int {
	optionsTable := fmt.Sprintf("`%soptions`", prefix)

	// Delete expired transients and their timeout counterparts
	res, err := db.Exec(fmt.Sprintf(`
		DELETE a, b FROM %s a
		INNER JOIN %s b ON b.option_name = CONCAT('_transient_timeout_', SUBSTRING(a.option_name, 12))
		WHERE a.option_name LIKE '_transient_%%'
		AND a.option_name NOT LIKE '_transient_timeout_%%'
		AND b.option_value < UNIX_TIMESTAMP()
	`, optionsTable, optionsTable))
	if err != nil {
		// Fallback: delete just the expired timeout markers
		res, err = db.Exec(fmt.Sprintf(`
			DELETE FROM %s
			WHERE option_name LIKE '_transient_timeout_%%'
			AND option_value < UNIX_TIMESTAMP()
		`, optionsTable))
		if err != nil {
			slog.Warn("Failed to clear transients", "table", optionsTable, "error", err)
			return 0
		}
	}

	affected, _ := res.RowsAffected()
	return int(affected)
}
