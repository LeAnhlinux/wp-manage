package fail2ban

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type JailStatus struct {
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Filter    string   `json:"filter"`
	BanTime   int      `json:"ban_time"`
	MaxRetry  int      `json:"max_retry"`
	BannedIPs []string `json:"banned_ips"`
	TotalBans int      `json:"total_bans"`
}

// IsServiceRunning returns true if fail2ban systemd service is active.
func IsServiceRunning() bool {
	out, err := exec.Command("systemctl", "is-active", "fail2ban").Output()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// ToggleService starts or stops the fail2ban systemd service.
func ToggleService(enable bool) error {
	var cmd *exec.Cmd
	if enable {
		cmd = exec.Command("systemctl", "enable", "--now", "fail2ban")
	} else {
		cmd = exec.Command("systemctl", "disable", "--now", "fail2ban")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// GetStatus returns fail2ban status including all jails.
// Returns empty slice (not error) when fail2ban is stopped.
func GetStatus() ([]JailStatus, error) {
	out, err := exec.Command("fail2ban-client", "status").Output()
	if err != nil {
		return nil, fmt.Errorf("fail2ban-client status: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	var jailNames []string
	for _, line := range lines {
		if strings.Contains(line, "Jail list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				names := strings.Split(strings.TrimSpace(parts[1]), ",")
				for _, n := range names {
					n = strings.TrimSpace(n)
					if n != "" {
						jailNames = append(jailNames, n)
					}
				}
			}
		}
	}

	var jails []JailStatus
	for _, name := range jailNames {
		jail := JailStatus{Name: name, Enabled: true}

		jailOut, err := exec.Command("fail2ban-client", "status", name).Output()
		if err == nil {
			for _, line := range strings.Split(string(jailOut), "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "Banned IP list:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						jail.BannedIPs = strings.Fields(strings.TrimSpace(parts[1]))
					}
				}
				if strings.Contains(line, "Currently banned:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						jail.TotalBans, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
					}
				}
			}
		}

		if jail.BannedIPs == nil {
			jail.BannedIPs = []string{}
		}
		jails = append(jails, jail)
	}

	return jails, nil
}

// UnbanIP unbans an IP from a specific jail.
func UnbanIP(jail, ip string) error {
	out, err := exec.Command("fail2ban-client", "set", jail, "unbanip", ip).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unban %s from %s: %s", ip, jail, string(out))
	}
	return nil
}

// UpdateJail updates jail configuration (enable/disable, ban time, max retry).
func UpdateJail(jail string, enabled *bool, banTime, maxRetry int) error {
	if enabled != nil {
		if *enabled {
			exec.Command("fail2ban-client", "start", jail).Run()
		} else {
			exec.Command("fail2ban-client", "stop", jail).Run()
		}
	}
	if banTime > 0 {
		exec.Command("fail2ban-client", "set", jail, "bantime", strconv.Itoa(banTime)).Run()
	}
	if maxRetry > 0 {
		exec.Command("fail2ban-client", "set", jail, "maxretry", strconv.Itoa(maxRetry)).Run()
	}
	return nil
}

// StatusJSON returns full status as JSON: { service_running, jails }.
// If service is stopped, jails will be empty but no error is returned.
func StatusJSON() (json.RawMessage, error) {
	running := IsServiceRunning()
	var jails []JailStatus
	if running {
		var err error
		jails, err = GetStatus()
		if err != nil {
			// Service running but client failed — still report running=true
			jails = []JailStatus{}
		}
	} else {
		jails = []JailStatus{}
	}
	return json.Marshal(map[string]interface{}{
		"service_running": running,
		"jails":           jails,
	})
}
