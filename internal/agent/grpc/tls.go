package grpc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

// generateSelfSignedTLS creates an in-memory self-signed TLS certificate for the
// agent HTTP server.  A new certificate is generated on every agent startup so that
// no private key material is ever written to disk.
//
// The certificate is valid for 10 years and covers:
//   - localhost / 127.0.0.1 (local access)
//   - The server's local IP addresses (detected at runtime)
//
// The API server uses InsecureSkipVerify when connecting to the agent because the
// certificate is self-signed. This still provides encryption in transit and prevents
// passive eavesdropping — it does NOT prevent active MITM if the agent IP is
// compromised. Full mTLS with pinned certs is on the roadmap.
func generateSelfSignedTLS() (tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Collect all local IPs to include in the SAN list
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok {
				ips = append(ips, ipNet.IP)
			}
		}
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"WP Manage Agent"},
			CommonName:   "wp-manage-agent",
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// newAgentTLSConfig returns a *tls.Config suitable for the agent's HTTPS server.
func newAgentTLSConfig() (*tls.Config, error) {
	cert, err := generateSelfSignedTLS()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
