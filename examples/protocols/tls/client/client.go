package main

import (
	"crypto/tls"
	// "crypto/x509"
	// "encoding/pem"
	"log"
	"net"
	"time"

	"github.com/lesismal/arpc"
)

var rsaCertPEM = []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUD0UWXQ7oJnfCT3gomzzpTkENuGYwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMDEyMDcwODIzMTZaFw0zMDEy
MDUwODIzMTZaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQCtfbA6OByWdWUmwtlnpsTKfXbpPXFUkgTgcGvxXys4
/WHtbz1Zc4BGaukYIwRI56kzN8cmVTmEs9Cx+GQ35RZw4Y3VkuK9LZUuN/oGADTp
ZNRaVikvrTBgwMV0RuZbKWN1RpxDu6Oqs0i28yyviRTxLzBZrDUBr12DMFsL6uFr
4dRwVYVa1WN+p0PMwx/U8NNQl/RzoYUxBybUpqvx0kKRa6RhfE9MptIp4kNGAtzg
RHTeXhPClpsGTmerRnJUgqzLo4XxdcPevbJJ5uODS5G6mDIgbQdYOiRHXQJMdTca
5Ft6L3++FCTGPLcmb6EovU4xgs2PKk+ZkoYKvYqnofGpAgMBAAGjUzBRMB0GA1Ud
DgQWBBTo1zc0rYW5nt7DJp73C+JzPzWFYTAfBgNVHSMEGDAWgBTo1zc0rYW5nt7D
Jp73C+JzPzWFYTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBG
L0S/cRqIc78gGmhx/qqFXPnSfI5QlkTXLwOUZhbeBvpOZgS8MNAkUkWPNNSehif/
rP0fkzZFfnLIHnaqotgrfC0AitY/9VuDvEVQz/xBiuDuQZQDmmTKYLD/hZUr6Neu
jECc6Y6Cea//VD5pGt5LjXmk0XwcQ+g+zcsV3vKoikHUnB396+4aMZOGnNPwRrQF
3ywtj8nzJTZ9K923etQfZyjIRXMhzUMzni7HApEG/X0SWQLup9nK7fvtMLWWwHZ1
B8fTVnDQGdgzGi8PW9NCtgd4ylsHWh2JAkt109ZveG7W+SkCxDoTwp+izsFRlHUq
AJBEaPy+IMNsudg+ZcWm
-----END CERTIFICATE-----
`)

var rsaKeyPEM = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEArX2wOjgclnVlJsLZZ6bEyn126T1xVJIE4HBr8V8rOP1h7W89
WXOARmrpGCMESOepMzfHJlU5hLPQsfhkN+UWcOGN1ZLivS2VLjf6BgA06WTUWlYp
L60wYMDFdEbmWyljdUacQ7ujqrNItvMsr4kU8S8wWaw1Aa9dgzBbC+rha+HUcFWF
WtVjfqdDzMMf1PDTUJf0c6GFMQcm1Kar8dJCkWukYXxPTKbSKeJDRgLc4ER03l4T
wpabBk5nq0ZyVIKsy6OF8XXD3r2ySebjg0uRupgyIG0HWDokR10CTHU3GuRbei9/
vhQkxjy3Jm+hKL1OMYLNjypPmZKGCr2Kp6HxqQIDAQABAoIBACN2UZNU7OMEVAy8
P1wkho0tYCUE3il/P2fxEt9fqKIZiO7TkiK6rTm3mLXKUpHkaH2DpT18pikt6Da4
oyOZvCCOukMxpw8sRhYQcxbO7AHZDl74xaptKDperP27kFKJ/z51lHNz41x9ERv0
UOoAhztVffiWbq9NfTvXooSpGjLGvAJFD4XwqMcKCL6att591fPBUEm37OaClKAl
lzX4SIHFYHTl3wumGozYA+re5SXGH8Urz+Sde8YSHcn6jrjjtdm1Vt53FGyxg/uK
mSo2bRsNjkDU1sqB+a+JU/OBnoSSN/90GYj+NSTSKeGxdPV9N+CdbrhMRknUOGKT
rBoTyFECgYEA5W/Zqx/PvMhQoTospf2j2FY5nU8Mx106YUoTxb/hA21jfa075fZx
7oYT0Qs74XTVg1i3l3p9baD9Z4EwzjxM9lgboszI5f50bwRqX6TzdJeIZHqvLcVP
fuVTCpuIfU8V4zAVAD2hCHDiRsuYRxUPQmmiKaNXQDIYdGSm8JsnxjMCgYEAwZOw
Lhrp2e7tYVtBSvW17M5aScjFiX7Oxo8xwVvrAfLrv8V7k+KbM4phC4O5c6XsqLAB
qK9/EL0v54x1Bo+QUazgeJawBaEvfUUOC3V5ry/aWKXCSmmzTJs8RnxNsLv4HoQe
woT27hr3Pl2Hfz9JMdAvO+3BMzb3KPx3GF8wNLMCgYAk/YF0a26MmycUt1JXeKsf
x9cGG6aNxeQRp2XErgjTCqHNs05C5xa7Q/aR72O6F6IMyRLgYykxsZDpTRTXSzWF
SfM6rhV9ryaKd4XG4cs2cu/Uc0sm7/a/GK3oueapfUSkGi5omYcK21g/3bcxTp3l
MS6p0+HPQcRbj5ayl/EzrQKBgBVbtUGxCIJaQWjPh8m8iKEjN4USmPENw8TWwdei
y7BAXFChenwbsaIjL4f0tb6T3SPTn6s8CdoP9bwnnDXoGzVXzMChZ7SHT1UUDHOp
N47jycSkLWbGeNkH+8OPLYdFhh/f1gECaLhm00bXTP72PZ44aS3Ekt+SvfyQtpdC
0W/PAoGBAK1HYxk7pRcmU15Dqin/HL/pcnEApGGdwsJMhWZdC/h+VbaFryEOycnF
P0LWNqw/ATS2H4J+MWF54PQTbj1edJL/vXoeXbICmXHu80lyFWsFPh4H364GTtpR
PqDPZTQEc6P2lyAIYFjp5mE+9+waltjI/UQj1ZtnnBVY4p2dtMfp
-----END RSA PRIVATE KEY-----

`)

func main() {
	cert, err := tls.X509KeyPair(rsaCertPEM, rsaKeyPEM)
	if err != nil {
		log.Fatalf("tls.X509KeyPair failed: %v", err)
	}
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	client, err := arpc.NewClient(func() (net.Conn, error) {
		return tls.Dial("tcp", "localhost:8888", tlsConfig)
	})
	if err != nil {
		panic(err)
	}
	defer client.Stop()

	req := "hello"
	rsp := ""
	err = client.Call("/echo", &req, &rsp, time.Second*5)
	if err != nil {
		log.Fatalf("Call failed: %v", err)
	} else {
		log.Printf("Call Response: \"%v\"", rsp)
	}
}
