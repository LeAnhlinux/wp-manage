// Package providers defines the PanelProvider abstraction used by the Transfer Tool.
// Each provider implements discovery and extraction for a specific hosting panel.
// This package must NOT import the parent `transfer` package to avoid import cycles.
// Instead it defines the SSHClient interface that transfer.SSHClient satisfies.
package providers

import (
	"io"
	"net"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// SSHClient is the interface that transfer.SSHClient satisfies.
// Defined here to avoid an import cycle between `transfer` and `providers`.
type SSHClient interface {
	Run(cmd string) (string, error)
	RunQuiet(cmd string) string
	// RunDirSize is like RunQuiet but uses a longer timeout (10 min) for `du` commands
	// that may scan millions of inodes across large WordPress site directories.
	RunDirSize(cmd string) string
	ReadFile(path string) (string, error)
	FileExists(path string) bool
	DirSize(path string) int64
	DownloadFile(remotePath, localPath string) error
	StreamPipe(cmd string, dst io.Writer) error
	DialTCP(network, addr string) (net.Conn, error)
	RsyncFromSource(srcDir, dstDir, sshKey, sshPassword string, progress func(pct int)) error
	Host() string
	Close()
}

// ProgressFn is called periodically with a 0-100 percentage during long operations.
type ProgressFn func(pct int)

// PanelProvider is the interface every hosting panel adapter must implement.
// Phase 1: aaPanel. Phase 2+: cPanel, Plesk, CyberPanel, DirectAdmin.
type PanelProvider interface {
	// Name returns a human-readable name e.g. "aaPanel v7.0.5".
	Name() string

	// Detect returns true if the source server appears to run this panel.
	// Should be fast (1-2 SSH commands).
	Detect(ssh SSHClient) (bool, error)

	// Discover scans the source server and returns all found WordPress sites.
	// This includes domains, PHP version, DB credentials, SSL info, and disk size.
	Discover(ssh SSHClient) ([]models.DiscoveredSiteInfo, error)

	// StreamDB dumps the site's database over the SSH tunnel to the given writer.
	// No temp file is created on the source — the dump is streamed directly.
	StreamDB(ssh SSHClient, site *models.DiscoveredSiteInfo, dst io.Writer) error

	// FetchSSL downloads the site's SSL certificate and key from the source to local paths.
	FetchSSL(ssh SSHClient, ssl *models.TransferSSLInfo, localCertPath, localKeyPath string) error
}

// Registry maps panel type strings to provider constructors.
var Registry = map[string]func() PanelProvider{
	"aapanel": func() PanelProvider { return &AAPanelProvider{} },
	"cpanel":  func() PanelProvider { return &CPanelProvider{} },
}

// Get returns the provider for a given panel type, or nil if not found.
func Get(panelType string) PanelProvider {
	if fn, ok := Registry[panelType]; ok {
		return fn()
	}
	return nil
}
