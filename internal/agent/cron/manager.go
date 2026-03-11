package cron

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	CronFile = "/etc/cron.d/wp-manage"
	LogDir   = "/var/log/wp-manage-cron"
)

// EnsureCronInstalled installs the cron daemon if not present
func EnsureCronInstalled() error {
	if _, err := exec.LookPath("crontab"); err == nil {
		return nil // already installed
	}
	out, err := exec.Command("apt-get", "install", "-y", "cron").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install cron: %s: %w", string(out), err)
	}
	exec.Command("systemctl", "enable", "--now", "cron").Run()
	return nil
}

// AddEntry adds or updates a job entry in /etc/cron.d/wp-manage.
// Log output goes to /var/log/wp-manage-cron/{jobID}.log
func AddEntry(jobID uint64, user, schedule, command, comment string) error {
	if err := EnsureCronInstalled(); err != nil {
		return err
	}
	os.MkdirAll(LogDir, 0755)

	logFile := fmt.Sprintf("%s/%d.log", LogDir, jobID)

	// Load and filter existing lines, removing this job's old entry
	lines := filterJobLines(readJobLines(), jobID)

	// Append new entry — comment marks the job ID for future edits/removes
	marker := jobMarker(jobID)
	if comment != "" {
		lines = append(lines, fmt.Sprintf("# %s %s", comment, marker))
	} else {
		lines = append(lines, fmt.Sprintf("# cron job %d %s", jobID, marker))
	}
	lines = append(lines, fmt.Sprintf("%s %s %s >> %s 2>&1", schedule, user, command, logFile))

	return writeCronFile(lines)
}

// RemoveEntry removes a job's entry from /etc/cron.d/wp-manage
func RemoveEntry(jobID uint64) error {
	lines := filterJobLines(readJobLines(), jobID)
	return writeCronFile(lines)
}

// RunNow executes a command immediately (manual trigger) and appends output to the job log
func RunNow(jobID uint64, user, command string) (string, error) {
	os.MkdirAll(LogDir, 0755)
	logFile := fmt.Sprintf("%s/%d.log", LogDir, jobID)

	var cmd *exec.Cmd
	if user == "" || user == "root" {
		cmd = exec.Command("bash", "-c", command)
	} else {
		cmd = exec.Command("su", "-s", "/bin/bash", user, "-c", command)
	}
	out, err := cmd.CombinedOutput()

	// Append to log file with timestamp
	ts, _ := exec.Command("date", "+%Y-%m-%d %H:%M:%S").Output()
	entry := fmt.Sprintf("[%s Manual run]\n%s\n", strings.TrimSpace(string(ts)), string(out))
	f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if ferr == nil {
		f.WriteString(entry)
		f.Close()
	}

	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}

// GetLogs returns the last N lines of the job's log file
func GetLogs(jobID uint64, lines int) string {
	logFile := fmt.Sprintf("%s/%d.log", LogDir, jobID)
	if lines <= 0 {
		lines = 100
	}
	out, err := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logFile).Output()
	if err != nil {
		return "(no logs yet)"
	}
	return strings.TrimSpace(string(out))
}

// ---- internal helpers ----

func jobMarker(jobID uint64) string {
	return fmt.Sprintf("[wp-manage:job:%d]", jobID)
}

// readJobLines reads /etc/cron.d/wp-manage and returns only job lines (strips header)
func readJobLines() []string {
	content, err := os.ReadFile(CronFile)
	if err != nil {
		return nil
	}
	var result []string
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if t == "" ||
			t == "# Managed by WP Manage — do not edit manually" ||
			strings.HasPrefix(t, "SHELL=") ||
			strings.HasPrefix(t, "PATH=") {
			continue
		}
		result = append(result, line)
	}
	return result
}

// filterJobLines removes all lines belonging to jobID
func filterJobLines(lines []string, jobID uint64) []string {
	marker := jobMarker(jobID)
	var result []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, marker) {
			// This is the comment line — skip it and the next command line
			skip = true
			continue
		}
		if skip {
			skip = false
			continue
		}
		result = append(result, line)
	}
	return result
}

// writeCronFile writes job lines back to /etc/cron.d/wp-manage with the standard header
func writeCronFile(jobLines []string) error {
	header := "# Managed by WP Manage — do not edit manually\n" +
		"SHELL=/bin/bash\n" +
		"PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin\n"

	body := strings.Join(jobLines, "\n")
	if len(jobLines) > 0 {
		body += "\n"
	}
	return os.WriteFile(CronFile, []byte(header+"\n"+body), 0644)
}
