package database

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DBDatabase represents a MariaDB database
type DBDatabase struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	Charset   string `json:"charset"`
	Collation string `json:"collation"`
}

// ListDatabases returns all non-system databases
func ListDatabases() ([]DBDatabase, error) {
	out, err := exec.Command("mysql", "-N", "-e",
		`SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME
		 FROM information_schema.SCHEMATA
		 WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys')
		 ORDER BY SCHEMA_NAME`).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	var dbs []DBDatabase
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			db := DBDatabase{Name: parts[0], Charset: parts[1], Collation: parts[2]}
			// Get size
			sizeOut, _ := exec.Command("mysql", "-N", "-e",
				fmt.Sprintf(`SELECT ROUND(SUM(data_length+index_length)/1024/1024,2) FROM information_schema.TABLES WHERE TABLE_SCHEMA='%s'`, db.Name)).Output()
			db.Size = strings.TrimSpace(string(sizeOut)) + " MB"
			dbs = append(dbs, db)
		}
	}
	return dbs, nil
}

// CreateDatabase creates a new MariaDB database
func CreateDatabase(name, charset, collation string) error {
	if name == "" {
		return fmt.Errorf("database name is required")
	}
	if charset == "" {
		charset = "utf8mb4"
	}
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s COLLATE %s", name, charset, collation)
	if out, err := exec.Command("mysql", "-e", sql).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create database: %s: %w", string(out), err)
	}
	return nil
}

// DropDatabase drops a MariaDB database
func DropDatabase(name string) error {
	if name == "" {
		return fmt.Errorf("database name is required")
	}
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name)
	if out, err := exec.Command("mysql", "-e", sql).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to drop database: %s: %w", string(out), err)
	}
	return nil
}

// CreateUser creates a new MariaDB user with specified privileges
func CreateUser(username, password, database string, privileges []string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	// Generate password if not provided
	if password == "" {
		out, err := exec.Command("openssl", "rand", "-base64", "16").Output()
		if err != nil {
			return fmt.Errorf("failed to generate password: %w", err)
		}
		password = strings.TrimSpace(string(out))
	}

	// Create user
	createSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'", username, password)
	if out, err := exec.Command("mysql", "-e", createSQL).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create user: %s: %w", string(out), err)
	}

	// Grant privileges
	if database != "" {
		privStr := "ALL PRIVILEGES"
		if len(privileges) > 0 {
			privStr = strings.Join(privileges, ", ")
		}
		grantSQL := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'localhost'", privStr, database, username)
		if out, err := exec.Command("mysql", "-e", grantSQL).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to grant privileges: %s: %w", string(out), err)
		}
	}

	// Flush privileges
	exec.Command("mysql", "-e", "FLUSH PRIVILEGES").Run()
	return nil
}

// DropUser drops a MariaDB user
func DropUser(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	dropSQL := fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost'", username)
	if out, err := exec.Command("mysql", "-e", dropSQL).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to drop user: %s: %w", string(out), err)
	}

	exec.Command("mysql", "-e", "FLUSH PRIVILEGES").Run()
	return nil
}

// DBUser represents a MariaDB user
type DBUser struct {
	Username  string   `json:"username"`
	Host      string   `json:"host"`
	Databases []string `json:"databases"`
}

// ListUsers returns all non-system MariaDB users with their databases
func ListUsers() ([]DBUser, error) {
	out, err := exec.Command("mysql", "-N", "-e",
		"SELECT User, Host FROM mysql.user WHERE User NOT IN ('root','mysql','mariadb.sys','debian-sys-maint') AND User != '' ORDER BY User").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var users []DBUser
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			user := DBUser{Username: parts[0], Host: parts[1]}
			// Get databases this user has access to
			dbOut, _ := exec.Command("mysql", "-N", "-e",
				fmt.Sprintf("SELECT DISTINCT TABLE_SCHEMA FROM information_schema.SCHEMA_PRIVILEGES WHERE GRANTEE = \"'%s'@'%s'\" AND TABLE_SCHEMA NOT IN ('information_schema','mysql','performance_schema','sys')",
					user.Username, user.Host)).Output()
			for _, db := range strings.Split(strings.TrimSpace(string(dbOut)), "\n") {
				if db != "" {
					user.Databases = append(user.Databases, db)
				}
			}
			users = append(users, user)
		}
	}
	return users, nil
}

// GetConfig reads current MariaDB configuration values
func GetConfig() (map[string]interface{}, error) {
	out, err := exec.Command("mysql", "-N", "-e",
		"SELECT @@max_connections, @@innodb_buffer_pool_size, @@slow_query_log, @@long_query_time").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	cfg := map[string]interface{}{
		"max_connections":  150,
		"innodb_buffer_mb": 256,
		"query_cache_mb":   64,
		"slow_query_log":   true,
		"slow_query_time":  2,
		"custom_config":    "",
	}

	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) >= 4 {
		if v, err := strconv.Atoi(parts[0]); err == nil {
			cfg["max_connections"] = v
		}
		if v, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			cfg["innodb_buffer_mb"] = int(v / 1024 / 1024)
		}
		cfg["slow_query_log"] = parts[2] == "1"
		if v, err := strconv.ParseFloat(parts[3], 64); err == nil {
			cfg["slow_query_time"] = int(v)
		}
	}
	return cfg, nil
}

// UpdateConfig updates MariaDB server configuration
func UpdateConfig(maxConnections, innodbBufferMB, queryCacheMB int, slowQueryLog bool, slowQueryTime int, customConfig string) error {
	// Build config content
	var configLines []string
	configLines = append(configLines, "[mysqld]")

	if maxConnections > 0 {
		configLines = append(configLines, fmt.Sprintf("max_connections = %d", maxConnections))
	}
	if innodbBufferMB > 0 {
		configLines = append(configLines, fmt.Sprintf("innodb_buffer_pool_size = %dM", innodbBufferMB))
	}
	if queryCacheMB > 0 {
		configLines = append(configLines, fmt.Sprintf("query_cache_size = %dM", queryCacheMB))
		configLines = append(configLines, "query_cache_type = 1")
	}
	if slowQueryLog {
		configLines = append(configLines, "slow_query_log = 1")
		configLines = append(configLines, "slow_query_log_file = /var/log/mysql/slow-query.log")
		if slowQueryTime > 0 {
			configLines = append(configLines, fmt.Sprintf("long_query_time = %d", slowQueryTime))
		}
	}
	if customConfig != "" {
		configLines = append(configLines, customConfig)
	}

	// Write to custom config file
	configContent := strings.Join(configLines, "\n") + "\n"
	configPath := "/etc/mysql/mariadb.conf.d/99-wp-manage.cnf"
	if out, err := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' > %s", configContent, configPath)).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write config: %s: %w", string(out), err)
	}

	// Restart MariaDB
	if out, err := exec.Command("systemctl", "restart", "mariadb").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart MariaDB: %s: %w", string(out), err)
	}

	return nil
}
