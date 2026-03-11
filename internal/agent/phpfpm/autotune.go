package phpfpm

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type AutotuneResult struct {
	MaxChildren     int    `json:"max_children"`
	StartServers    int    `json:"start_servers"`
	MinSpareServers int    `json:"min_spare_servers"`
	MaxSpareServers int    `json:"max_spare_servers"`
	RAMAvailable    int    `json:"ram_available_mb"`
	Message         string `json:"message"`
}

// Autotune calculates optimal PHP-FPM pool settings based on available RAM
func Autotune(domain, phpVersion string) (*AutotuneResult, error) {
	// Get total RAM in MB
	ramMB, err := getTotalRAM()
	if err != nil {
		return nil, fmt.Errorf("failed to get RAM info: %w", err)
	}

	// Reserve 40% for OS + MariaDB + Nginx + Redis
	availableRAM := int(float64(ramMB) * 0.6)

	// Average PHP-FPM process uses ~64MB
	avgProcessMB := 64
	maxChildren := availableRAM / avgProcessMB

	// Set reasonable bounds
	if maxChildren < 2 {
		maxChildren = 2
	}
	if maxChildren > 100 {
		maxChildren = 100
	}

	startServers := maxChildren / 4
	if startServers < 1 {
		startServers = 1
	}

	minSpare := startServers
	maxSpare := maxChildren / 2
	if maxSpare < minSpare {
		maxSpare = minSpare + 1
	}

	result := &AutotuneResult{
		MaxChildren:     maxChildren,
		StartServers:    startServers,
		MinSpareServers: minSpare,
		MaxSpareServers: maxSpare,
		RAMAvailable:    availableRAM,
		Message:         fmt.Sprintf("Optimized for %dMB available RAM (%dMB total)", availableRAM, ramMB),
	}

	// Apply settings to pool config
	poolFile := fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", phpVersion, domain)
	if _, err := os.Stat(poolFile); os.IsNotExist(err) {
		// Try www.conf as fallback
		poolFile = fmt.Sprintf("/etc/php/%s/fpm/pool.d/www.conf", phpVersion)
	}

	if _, err := os.Stat(poolFile); err == nil {
		if err := updatePoolConfig(poolFile, result); err != nil {
			return result, fmt.Errorf("calculated values but failed to apply: %w", err)
		}

		// Restart PHP-FPM
		svcName := fmt.Sprintf("php%s-fpm", phpVersion)
		if err := exec.Command("systemctl", "restart", svcName).Run(); err != nil {
			slog.Warn("Failed to restart PHP-FPM", "service", svcName, "error", err)
		}
	}

	slog.Info("PHP-FPM autotuned", "domain", domain, "max_children", maxChildren, "ram_mb", ramMB)
	return result, nil
}

func getTotalRAM() (int, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err != nil {
					return 0, err
				}
				return kb / 1024, nil // Convert KB to MB
			}
		}
	}
	return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
}

func updatePoolConfig(poolFile string, result *AutotuneResult) error {
	data, err := os.ReadFile(poolFile)
	if err != nil {
		return err
	}

	content := string(data)

	replacements := map[string]int{
		"pm.max_children":      result.MaxChildren,
		"pm.start_servers":     result.StartServers,
		"pm.min_spare_servers": result.MinSpareServers,
		"pm.max_spare_servers": result.MaxSpareServers,
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, val := range replacements {
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
				lines[i] = fmt.Sprintf("%s = %d", key, val)
				break
			}
		}
	}

	return os.WriteFile(poolFile, []byte(strings.Join(lines, "\n")), 0644)
}
