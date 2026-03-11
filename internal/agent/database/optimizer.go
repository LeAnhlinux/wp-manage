package database

import (
	"database/sql"
	"fmt"
	"log/slog"

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

	// 1. Optimize all tables
	result.TablesOptimized = optimizeTables(db, task.DBName)

	// 2. Calculate space saved (approximate from fragmented space)
	result.SpaceSaved = getFragmentedSpace(db, task.DBName)

	// 3. Clean post revisions (keep last 5 per post)
	result.RevisionsClean = cleanRevisions(db)

	// 4. Clear expired transients
	result.TransientsClear = clearTransients(db)

	slog.Info("DB optimization completed",
		"db", task.DBName,
		"tables", result.TablesOptimized,
		"space_saved", result.SpaceSaved,
		"revisions_cleaned", result.RevisionsClean,
		"transients_cleared", result.TransientsClear,
	)

	return result
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

func cleanRevisions(db *sql.DB) int {
	// Delete all revisions except the last 5 per post
	// Uses wp_ prefix by default (most common)
	res, err := db.Exec(`
		DELETE FROM wp_posts WHERE post_type = 'revision'
		AND ID NOT IN (
			SELECT ID FROM (
				SELECT p.ID FROM wp_posts p
				WHERE p.post_type = 'revision'
				ORDER BY p.post_date DESC
				LIMIT 999999999
			) AS keep_revisions
			WHERE ID IN (
				SELECT sub.ID FROM (
					SELECT r.ID,
						ROW_NUMBER() OVER (PARTITION BY r.post_parent ORDER BY r.post_date DESC) as rn
					FROM wp_posts r WHERE r.post_type = 'revision'
				) sub WHERE sub.rn <= 5
			)
		)
	`)
	if err != nil {
		// Try simpler approach: delete revisions older than 30 days
		res, err = db.Exec(`
			DELETE FROM wp_posts
			WHERE post_type = 'revision'
			AND post_date < DATE_SUB(NOW(), INTERVAL 30 DAY)
		`)
		if err != nil {
			slog.Warn("Failed to clean revisions", "error", err)
			return 0
		}
	}

	affected, _ := res.RowsAffected()
	return int(affected)
}

func clearTransients(db *sql.DB) int {
	// Delete expired transients from wp_options
	res, err := db.Exec(`
		DELETE a, b FROM wp_options a
		INNER JOIN wp_options b ON b.option_name = CONCAT('_transient_timeout_', SUBSTRING(a.option_name, 12))
		WHERE a.option_name LIKE '_transient_%'
		AND a.option_name NOT LIKE '_transient_timeout_%'
		AND b.option_value < UNIX_TIMESTAMP()
	`)
	if err != nil {
		// Simpler approach: just delete expired timeout entries
		res, err = db.Exec(`
			DELETE FROM wp_options
			WHERE option_name LIKE '_transient_timeout_%'
			AND option_value < UNIX_TIMESTAMP()
		`)
		if err != nil {
			slog.Warn("Failed to clear transients", "error", err)
			return 0
		}
	}

	affected, _ := res.RowsAffected()
	return int(affected)
}
