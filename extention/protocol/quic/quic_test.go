// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package quic

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"testing"
)

func TestAll(t *testing.T) {
	addr := "localhost:15678"
	ln, err := Listen(addr, generateTLSConfig())
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		nread, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		conn.Write(buf[:nread])
	}()

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	conn, err := Dial(addr, tlsConf, nil, 0)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer conn.Close()
	str := "hello"
	nwrite, err := conn.Write([]byte(str))
	if err != nil || nwrite != len(str) {
		t.Fatalf("failed to listen: %v", err)
	}
	buf, err := ioutil.ReadAll(conn)
	if err != nil || string(buf) != str {
		t.Fatalf("failed to listen: %v", err)
	}
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
