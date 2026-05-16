package providers

import (
	"io"

	"github.com/gofiber-team/wp-manage/internal/models"
)

// CPanelProvider is a Phase 2 stub — not yet implemented.
// Implements PanelProvider so the registry can include it.
type CPanelProvider struct{}

func (p *CPanelProvider) Name() string { return "cPanel (stub)" }

func (p *CPanelProvider) Detect(ssh SSHClient) (bool, error) {
	out := ssh.RunQuiet("test -d /usr/local/cpanel && echo yes || echo no")
	return out == "yes", nil
}

func (p *CPanelProvider) Discover(ssh SSHClient) ([]models.DiscoveredSiteInfo, error) {
	return nil, nil // Phase 2: not yet implemented
}

func (p *CPanelProvider) StreamDB(ssh SSHClient, site *models.DiscoveredSiteInfo, dst io.Writer) error {
	return nil // Phase 2 stub
}

func (p *CPanelProvider) FetchSSL(ssh SSHClient, ssl *models.TransferSSLInfo, localCert, localKey string) error {
	return nil // Phase 2 stub
}
