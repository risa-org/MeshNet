package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

func generateSelfSignedCert(
	pubkey ed25519.PublicKey,
	privkey ed25519.PrivateKey,
) (*tls.Certificate, error) {
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Meshnet Node",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		pubkey,
		privkey,
	)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privkey)
	if err != nil {
		return nil, err
	}

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	cert, err := tls.X509KeyPair(certPEM, privKeyPEM)
	if err != nil {
		return nil, err
	}

	return &cert, nil
}
