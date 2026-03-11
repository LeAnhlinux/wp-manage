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

// GetStatus returns fail2ban status including all jails
func GetStatus() ([]JailStatus, error) {
	// Get jail list
	out, err := exec.Command("fail2ban-client", "status").Output()
	if err != nil {
		return nil, fmt.Errorf("fail2ban-client status: %w", err)
	}

	// Parse jail list from output
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

		// Get per-jail status
		jailOut, err := exec.Command("fail2ban-client", "status", name).Output()
		if err == nil {
			jailStr := string(jailOut)
			// Parse banned IPs
			for _, line := range strings.Split(jailStr, "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "Banned IP list:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						ips := strings.Fields(strings.TrimSpace(parts[1]))
						jail.BannedIPs = ips
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

// UnbanIP unbans an IP from a specific jail
func UnbanIP(jail, ip string) error {
	out, err := exec.Command("fail2ban-client", "set", jail, "unbanip", ip).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unban %s from %s: %s", ip, jail, string(out))
	}
	return nil
}

// UpdateJail updates jail configuration
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

// StatusJSON returns status as JSON bytes
func StatusJSON() (json.RawMessage, error) {
	jails, err := GetStatus()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"jails": jails})
}
