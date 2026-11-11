package sftp

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/gofiber-team/wp-manage/pkg/utils"
)

// siteUser derives the Linux username for a WordPress site (must match site/manager.go).
func siteUser(domain string) string {
	return utils.SiteLinuxUser(domain)
}

const sshdConfigPath = "/etc/ssh/sshd_config"
const sftpChrootBase = "/opt/sftp"
const fstabPath = "/etc/fstab"

// CreateUser creates a chrooted SFTP user using a bind-mount approach.
//
// The chroot jail is at /opt/sftp/{username} (owned root:root — required by sshd).
// The site's web root /var/www/{domain} is bind-mounted into the chroot at
// /opt/sftp/{username}/www so the SFTP user can access files without touching
// the original ownership.
//
// The SFTP user is added to the site's dedicated Linux group (wp_{domain}) so they
// can read/write site files (per-site isolation model).
func CreateUser(username, password, homeDir, domain string) error {
	slog.Info("Creating SFTP user", "username", username, "domain", domain)

	// Derive the site's Linux group (must match the group created by site/manager.go)
	sitePkg := siteUser(domain)

	// Ensure the sftpusers group exists
	exec.Command("groupadd", "-f", "sftpusers").Run()

	// Build paths
	chrootDir := fmt.Sprintf("%s/%s", sftpChrootBase, username)
	webMountPoint := fmt.Sprintf("%s/www", chrootDir)
	webRoot := fmt.Sprintf("/var/www/%s", domain)

	// Create the system user. Home is set to /www (relative to chroot).
	// Add to sftpusers (SFTP group) and the site's Linux group (so WP files are accessible).
	cmd := exec.Command("useradd",
		"-g", "sftpusers",
		"-G", sitePkg,
		"-d", "/www",
		"-s", "/usr/sbin/nologin",
		username,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "already exists") {
			return fmt.Errorf("failed to create user: %s", string(out))
		}
		// User exists — ensure they are in the site group
		exec.Command("usermod", "-aG", sitePkg, username).Run()
	}

	// Set password
	if password != "" {
		cmd = exec.Command("chpasswd")
		cmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", username, password))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set password: %s", string(out))
		}
	}

	// Create chroot jail directory — must be owned root:root for sshd chroot.
	// This is separate from /var/www/{domain}, so we're NOT breaking www-data access.
	if err := os.MkdirAll(chrootDir, 0755); err != nil {
		return fmt.Errorf("failed to create chroot dir: %w", err)
	}
	exec.Command("chown", "root:root", chrootDir).Run()
	exec.Command("chmod", "755", chrootDir).Run()

	// Create the bind-mount target directory inside the chroot.
	if err := os.MkdirAll(webMountPoint, 0755); err != nil {
		return fmt.Errorf("failed to create www mount point: %w", err)
	}

	// Ensure the web root exists (it may not yet if site is still provisioning)
	if err := os.MkdirAll(webRoot, 0755); err != nil {
		slog.Warn("Web root may not exist yet", "path", webRoot, "error", err)
	}

	// Grant group-write on the site's public dir so the SFTP user can upload files.
	// The site files are owned by wp_{domain}:wp_{domain} — SFTP user is in that group.
	publicDir := fmt.Sprintf("/var/www/%s/public", domain)
	exec.Command("find", publicDir, "-type", "d", "-exec", "chmod", "g+w", "{}", "+").Run()

	// Bind-mount the site web root into the chroot.
	// This preserves the original /var/www/{domain} ownership (www-data) intact.
	if out, err := exec.Command("mount", "--bind", webRoot, webMountPoint).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bind mount %s → %s: %s", webRoot, webMountPoint, string(out))
	}

	// Add fstab entry so the bind-mount survives reboots.
	fstabLine := fmt.Sprintf("%s %s none bind 0 0", webRoot, webMountPoint)
	if err := ensureFstabEntry(fstabLine); err != nil {
		slog.Warn("Failed to add fstab entry", "error", err)
	}

	// Configure sshd — ChrootDirectory is the chroot jail, not the web root.
	if err := ensureSFTPConfig(username, chrootDir); err != nil {
		return fmt.Errorf("failed to configure sshd: %w", err)
	}

	// Reload sshd to apply the new Match User block.
	if out, err := exec.Command("systemctl", "reload", "sshd").CombinedOutput(); err != nil {
		// Some distros use "ssh" as the service name
		if out2, err2 := exec.Command("systemctl", "reload", "ssh").CombinedOutput(); err2 != nil {
			slog.Warn("Failed to reload sshd, trying restart",
				"reload_err", string(out), "reload_ssh_err", string(out2))
			exec.Command("systemctl", "restart", "sshd").Run()
		}
	}

	slog.Info("SFTP user created successfully", "username", username, "chroot", chrootDir)
	return nil
}

// DeleteUser removes an SFTP user, unmounts the bind-mount, cleans up fstab
// and the chroot directory.
func DeleteUser(username, domain string) error {
	slog.Info("Deleting SFTP user", "username", username)

	chrootDir := fmt.Sprintf("%s/%s", sftpChrootBase, username)
	webMountPoint := fmt.Sprintf("%s/www", chrootDir)

	// Lazy-unmount to avoid "target is busy" errors.
	exec.Command("umount", "-l", webMountPoint).Run()

	// Remove fstab entry (match by mount point path).
	removeFstabEntry(webMountPoint)

	// Remove the Linux user.
	cmd := exec.Command("userdel", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "does not exist") {
			slog.Warn("Failed to delete SFTP user", "error", string(out))
		}
	}

	// Remove sshd Match User block.
	if err := removeSFTPConfig(username); err != nil {
		slog.Warn("Failed to remove SFTP config", "error", err)
	}

	// Remove the chroot directory (now safe since the bind-mount is gone).
	os.RemoveAll(chrootDir)

	// Reload sshd
	exec.Command("systemctl", "reload", "sshd").Run()
	exec.Command("systemctl", "reload", "ssh").Run()

	slog.Info("SFTP user deleted successfully", "username", username)
	return nil
}

// ensureFstabEntry appends a bind-mount line to /etc/fstab if not already present.
func ensureFstabEntry(line string) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return err
	}
	// Check if mount point is already in fstab (use the full line to be precise)
	if strings.Contains(string(data), line) {
		return nil
	}
	f, err := os.OpenFile(fstabPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + line + "\n")
	return err
}

// removeFstabEntry removes lines from /etc/fstab that contain the given mount point.
func removeFstabEntry(mountPoint string) {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return
	}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		// Skip lines that reference this mount point
		if strings.Contains(line, mountPoint) {
			continue
		}
		lines = append(lines, line)
	}
	os.WriteFile(fstabPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// ensureSFTPConfig adds a Match User block to sshd_config for the given user.
func ensureSFTPConfig(username, chrootDir string) error {
	data, err := os.ReadFile(sshdConfigPath)
	if err != nil {
		return err
	}

	marker := fmt.Sprintf("# SFTP-USER: %s", username)
	if strings.Contains(string(data), marker) {
		return nil // Already configured
	}

	config := fmt.Sprintf(`
%s
Match User %s
    ChrootDirectory %s
    ForceCommand internal-sftp
    AllowTcpForwarding no
    X11Forwarding no
`, marker, username, chrootDir)

	f, err := os.OpenFile(sshdConfigPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(config)
	return err
}

// removeSFTPConfig removes the Match User block for the given user from sshd_config.
func removeSFTPConfig(username string) error {
	data, err := os.ReadFile(sshdConfigPath)
	if err != nil {
		return err
	}

	marker := fmt.Sprintf("# SFTP-USER: %s", username)
	if !strings.Contains(string(data), marker) {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	inBlock := false

	for _, line := range lines {
		if strings.TrimSpace(line) == marker {
			inBlock = true
			continue
		}
		if inBlock {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// End of block when we hit a non-indented line or another marker/Match block
			if strings.HasPrefix(line, "Match ") || strings.HasPrefix(line, "# SFTP-USER:") ||
				(!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")) {
				inBlock = false
				newLines = append(newLines, line)
			}
			continue
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(sshdConfigPath, []byte(strings.Join(newLines, "\n")), 0644)
}
