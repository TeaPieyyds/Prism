package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
)

func generateClientPublicKey() string {
	pk, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return ""
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&pk.PublicKey)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(pubDER)
}
