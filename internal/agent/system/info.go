package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	Hostname      string   `json:"hostname"`
	OS            string   `json:"os"`
	OSVersion     string   `json:"os_version"`
	CPUCores      int      `json:"cpu_cores"`
	RAMMB         int64    `json:"ram_mb"`
	DiskGB        int64    `json:"disk_gb"`
	PHPVersions   []string `json:"php_versions"`
	NginxVersion  string   `json:"nginx_version"`
	MariaDBVersion string  `json:"mariadb_version"`
}

type Metrics struct {
	CPUUsage      float64 `json:"cpu_usage"`
	RAMUsage      float64 `json:"ram_usage"`
	RAMUsedMB     float64 `json:"ram_used_mb"`
	RAMTotalMB    float64 `json:"ram_total_mb"`
	DiskUsage     float64 `json:"disk_usage"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	Load1m        float64 `json:"load_1m"`
	Load5m        float64 `json:"load_5m"`
	Load15m       float64 `json:"load_15m"`
}

func GetInfo() (*Info, error) {
	hostname, _ := os.Hostname()

	info := &Info{
		Hostname: hostname,
		CPUCores: runtime.NumCPU(),
	}

	// OS info
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			}
			if strings.HasPrefix(line, "VERSION_ID=") {
				info.OSVersion = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}

	// RAM
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, _ := strconv.ParseInt(fields[1], 10, 64)
					info.RAMMB = kb / 1024
				}
			}
		}
	}

	// Disk
	if out, err := exec.Command("df", "-BG", "--output=size", "/").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			sizeStr := strings.TrimSuffix(strings.TrimSpace(lines[1]), "G")
			info.DiskGB, _ = strconv.ParseInt(sizeStr, 10, 64)
		}
	}

	// PHP versions
	info.PHPVersions = getInstalledPHPVersions()

	// Nginx version
	if out, err := exec.Command("nginx", "-v").CombinedOutput(); err == nil {
		v := string(out)
		if idx := strings.Index(v, "nginx/"); idx >= 0 {
			info.NginxVersion = strings.Fields(v[idx:])[0]
		}
	}

	// MariaDB version
	if out, err := exec.Command("mariadb", "--version").Output(); err == nil {
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if strings.Contains(f, "MariaDB") || strings.Contains(f, "mariadb") {
				if i+1 < len(fields) {
					info.MariaDBVersion = strings.TrimSuffix(fields[i], ",")
				}
			}
		}
	}

	return info, nil
}

func GetMetrics() (*Metrics, error) {
	m := &Metrics{}

	// CPU usage
	if out, err := exec.Command("sh", "-c", `top -bn1 | grep "Cpu(s)" | awk '{print $2}'`).Output(); err == nil {
		m.CPUUsage, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	}

	// RAM
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available int64
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch {
			case strings.HasPrefix(line, "MemTotal:"):
				total, _ = strconv.ParseInt(fields[1], 10, 64)
			case strings.HasPrefix(line, "MemAvailable:"):
				available, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
		m.RAMTotalMB = float64(total) / 1024
		m.RAMUsedMB = float64(total-available) / 1024
		if total > 0 {
			m.RAMUsage = float64(total-available) / float64(total) * 100
		}
	}

	// Disk
	if out, err := exec.Command("df", "-B1", "--output=size,used,pcent", "/").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 3 {
				totalBytes, _ := strconv.ParseFloat(fields[0], 64)
				usedBytes, _ := strconv.ParseFloat(fields[1], 64)
				m.DiskTotalGB = totalBytes / (1024 * 1024 * 1024)
				m.DiskUsedGB = usedBytes / (1024 * 1024 * 1024)
				pctStr := strings.TrimSuffix(fields[2], "%")
				m.DiskUsage, _ = strconv.ParseFloat(pctStr, 64)
			}
		}
	}

	// Load average
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			m.Load1m, _ = strconv.ParseFloat(fields[0], 64)
			m.Load5m, _ = strconv.ParseFloat(fields[1], 64)
			m.Load15m, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// Uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			uptime, _ := strconv.ParseFloat(fields[0], 64)
			m.UptimeSeconds = int64(uptime)
		}
	}

	return m, nil
}

func getInstalledPHPVersions() []string {
	versions := []string{}
	candidates := []string{"7.4", "8.0", "8.1", "8.2", "8.3"}
	for _, v := range candidates {
		binary := fmt.Sprintf("php%s", v)
		if _, err := exec.LookPath(binary); err == nil {
			versions = append(versions, v)
		}
	}
	return versions
}
