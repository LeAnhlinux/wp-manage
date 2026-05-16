// Package transfer provides the SSH client used by the Transfer Tool to reach
// the source server (typically running aaPanel, cPanel, or another panel).
// The agent on the TARGET server does all SSH work — no agent is needed on the source.
package transfer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient wraps an *ssh.Client with convenience methods.
type SSHClient struct {
	client *ssh.Client
	host   string
	port   int
	user   string
}

// NewSSHClientPassword connects via password authentication.
func NewSSHClientPassword(host string, port int, user, password string) (*SSHClient, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // source server is user-supplied
		Timeout:         15 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s failed: %w", addr, err)
	}
	return &SSHClient{client: client, host: host, port: port, user: user}, nil
}

// NewSSHClientKey connects via PEM private key authentication.
func NewSSHClientKey(host string, port int, user, pemKey string) (*SSHClient, error) {
	signer, err := ssh.ParsePrivateKey([]byte(pemKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         15 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s failed: %w", addr, err)
	}
	return &SSHClient{client: client, host: host, port: port, user: user}, nil
}

// Dial creates an SSHClient using either key or password (key takes priority).
func Dial(host string, port int, user, sshKey, sshPassword string) (*SSHClient, error) {
	if sshKey != "" {
		return NewSSHClientKey(host, port, user, sshKey)
	}
	return NewSSHClientPassword(host, port, user, sshPassword)
}

// Close closes the underlying ssh.Client.
func (c *SSHClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Timeout constants — sized to the actual operation, not a single blanket value.
const (
	// defaultCmdTimeout: short read-only commands (cat, grep, test -e, hostname...).
	// Should complete almost instantly; 15s covers slow networks + high-load servers.
	defaultCmdTimeout = 15 * time.Second

	// dirSizeTimeout: `du -sb dir1 dir2 ...` across all site directories.
	// A server with 20 × 3 GB sites + millions of inodes can take several minutes.
	dirSizeTimeout = 10 * time.Minute
)

// runTimeout executes cmd on the remote host with a hard deadline.
// If the command doesn't finish within d, the session is closed and an error is returned.
func (c *SSHClient) runTimeout(cmd string, d time.Duration) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}

	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := sess.CombinedOutput(cmd)
		ch <- result{strings.TrimSpace(string(out)), err}
	}()

	select {
	case r := <-ch:
		sess.Close()
		return r.out, r.err
	case <-time.After(d):
		sess.Close() // unblocks CombinedOutput
		return "", fmt.Errorf("SSH command timed out after %v", d)
	}
}

func (c *SSHClient) Run(cmd string) (string, error) {
	return c.runTimeout(cmd, defaultCmdTimeout)
}

// RunQuiet executes a command; returns stdout only (ignores stderr) and no error on non-zero exit.
func (c *SSHClient) RunQuiet(cmd string) string {
	out, _ := c.runTimeout(cmd, defaultCmdTimeout)
	return out
}

// RunDirSize runs a du command with the longer dirSizeTimeout.
// Use this instead of RunQuiet for any `du` invocation that spans many/large directories.
func (c *SSHClient) RunDirSize(cmd string) string {
	out, _ := c.runTimeout(cmd, dirSizeTimeout)
	return out
}

// ReadFile reads a remote file's contents into a string.
func (c *SSHClient) ReadFile(path string) (string, error) {
	out, err := c.Run("cat " + shellQuote(path))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// FileExists returns true if the remote path exists.
func (c *SSHClient) FileExists(path string) bool {
	out, _ := c.Run("test -e " + shellQuote(path) + " && echo yes || echo no")
	return out == "yes"
}

// DirSize returns the total size in bytes of a remote directory using `du`.
func (c *SSHClient) DirSize(path string) int64 {
	out := c.RunDirSize(fmt.Sprintf("du -sb %s 2>/dev/null | awk '{print $1}'", shellQuote(path)))
	var size int64
	fmt.Sscanf(out, "%d", &size)
	return size
}

// DownloadFile streams a remote file to a local path using scp-like sftp.
// Uses the existing *ssh.Client SFTP channel under the hood.
func (c *SSHClient) DownloadFile(remotePath, localPath string) error {
	// Use `ssh cat` pipe + local file write — simpler than sftp for our use-case
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sess.Stdout = f
	return sess.Run("cat " + shellQuote(remotePath))
}

// StreamPipe runs a remote command and writes its stdout to the provided writer.
// Used for mysqldump streaming without a temp file on the source.
func (c *SSHClient) StreamPipe(cmd string, dst io.Writer) error {
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = dst
	var errBuf bytes.Buffer
	sess.Stderr = &errBuf
	if err := sess.Run(cmd); err != nil {
		return fmt.Errorf("%w: %s", err, errBuf.String())
	}
	return nil
}

// UploadStream sends data from a reader to a remote file path.
func (c *SSHClient) UploadStream(remotePath string, src io.Reader) error {
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = src
	return sess.Run("cat > " + shellQuote(remotePath))
}

// Host returns the remote host address.
func (c *SSHClient) Host() string { return c.host }

// RsyncFromSource runs rsync from the local agent machine, pulling files FROM the source
// over SSH. The SSH key material is written to a temp file for rsync's -e option.
// srcDir must end with '/' to rsync contents (not the directory itself).
func (c *SSHClient) RsyncFromSource(srcDir, dstDir, sshKey, sshPassword string, progress func(pct int)) error {
	// Build rsync command
	// We pull: rsync -az --partial --append-verify -e "ssh ..." user@host:srcDir dstDir/
	sshOpts := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	var keyFile string
	if sshKey != "" {
		// Write key to temp file
		f, err := os.CreateTemp("", "wpm-ssh-*.pem")
		if err != nil {
			return err
		}
		keyFile = f.Name()
		defer os.Remove(keyFile)
		if _, err := f.WriteString(sshKey); err != nil {
			return err
		}
		f.Close()
		if err := os.Chmod(keyFile, 0600); err != nil {
			return err
		}
		sshOpts += " -i " + keyFile
	}

	sshCmd := fmt.Sprintf("ssh -p %d %s", c.port, sshOpts)
	src := fmt.Sprintf("%s@%s:%s", c.user, c.host, srcDir)

	args := []string{
		"-az", "--partial", "--append-verify",
		"--progress",
		"-e", sshCmd,
		src,
		dstDir,
	}

	if sshPassword != "" {
		// sshpass -p <password> rsync <rsync-args>
		sshpassArgs := append([]string{"-p", sshPassword, "rsync"}, args...)
		cmd := exec.Command("sshpass", sshpassArgs...)
		return runWithProgress(cmd, progress)
	}

	cmd := exec.Command("rsync", args...)
	return runWithProgress(cmd, progress)
}

// runWithProgress runs a command and calls the progress callback with a rough percentage estimate.
func runWithProgress(cmd *exec.Cmd, progress func(pct int)) error {
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		pw.Close()
	}()

	// Parse rsync progress output heuristically
	buf := make([]byte, 4096)
	pct := 0
	for {
		n, err := pr.Read(buf)
		if n > 0 && progress != nil {
			line := string(buf[:n])
			// rsync outputs "X%" lines; parse roughly
			if idx := strings.LastIndex(line, "%"); idx > 0 {
				start := idx - 3
				if start < 0 {
					start = 0
				}
				var p int
				fmt.Sscanf(strings.TrimSpace(line[start:idx]), "%d", &p)
				if p > pct && p <= 100 {
					pct = p
					progress(pct)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return <-done
}

// Dial a TCP connection through the SSH tunnel — used for MySQL streaming.
func (c *SSHClient) DialTCP(network, addr string) (net.Conn, error) {
	return c.client.Dial(network, addr)
}

