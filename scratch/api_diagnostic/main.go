package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type DiagnosticResult struct {
	Target                 string
	DNSResolutionSuccess   bool
	ResolvedIPs            []string
	DNSError               string
	TCPConnectSuccess      bool
	TCPConnectError        string
	TLSHandshakeSuccess    bool
	TLSError               string
	NegotiatedTLSVersion   string
	NegotiatedCipherSuite  string
	ServerCertSubject      string
	ServerCertIssuer       string
	HTTPResponseReceived   bool
	HTTPStatusCode         int
	HTTPProtocol           string
	HTTPError              string
	CloudflareEdgeDetected bool
	CloudflareRayID        string
	CloudflareOriginDown   bool
	BackendHealthy         bool
	ResponseBody           string
}

func inspectEndpoint(targetURL string) DiagnosticResult {
	res := DiagnosticResult{Target: targetURL}

	host := "api.onusly.com"
	port := "443"

	// 1. DNS Resolution
	ips, err := net.LookupHost(host)
	if err != nil {
		res.DNSError = err.Error()
		return res
	}
	res.DNSResolutionSuccess = true
	res.ResolvedIPs = ips

	// 2. TCP Connectivity
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	rawConn, err := dialer.Dial("tcp", net.JoinHostPort(ips[0], port))
	if err != nil {
		res.TCPConnectError = err.Error()
		return res
	}
	res.TCPConnectSuccess = true
	defer rawConn.Close()

	// 3. TLS Handshake
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		res.TLSError = err.Error()
		return res
	}
	res.TLSHandshakeSuccess = true
	defer tlsConn.Close()

	cs := tlsConn.ConnectionState()
	switch cs.Version {
	case tls.VersionTLS13:
		res.NegotiatedTLSVersion = "TLS 1.3"
	case tls.VersionTLS12:
		res.NegotiatedTLSVersion = "TLS 1.2"
	default:
		res.NegotiatedTLSVersion = fmt.Sprintf("0x%04x", cs.Version)
	}
	res.NegotiatedCipherSuite = tls.CipherSuiteName(cs.CipherSuite)
	if len(cs.PeerCertificates) > 0 {
		res.ServerCertSubject = cs.PeerCertificates[0].Subject.CommonName
		res.ServerCertIssuer = cs.PeerCertificates[0].Issuer.CommonName
	}

	// 4. HTTP Request & Response
	transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		res.HTTPError = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "ONUSLY-SRE-Recovery-Probe/1.0")

	resp, err := client.Do(req)
	if err != nil {
		res.HTTPError = err.Error()
		return res
	}
	defer resp.Body.Close()

	res.HTTPResponseReceived = true
	res.HTTPStatusCode = resp.StatusCode
	res.HTTPProtocol = resp.Proto
	res.CloudflareRayID = resp.Header.Get("cf-ray")
	res.CloudflareEdgeDetected = resp.Header.Get("server") == "cloudflare" || res.CloudflareRayID != ""

	bodyBytes, _ := io.ReadAll(resp.Body)
	res.ResponseBody = string(bodyBytes)

	// 5. Backend Health vs Cloudflare 521 Discrimination
	if res.HTTPStatusCode == 521 {
		res.CloudflareOriginDown = true
		res.BackendHealthy = false
	} else if res.HTTPStatusCode == 200 {
		res.CloudflareOriginDown = false
		res.BackendHealthy = true
	} else if res.HTTPStatusCode == 401 {
		// 401 on protected endpoint means backend is alive and actively rejecting unauthenticated callers
		res.CloudflareOriginDown = false
		res.BackendHealthy = true
	}

	return res
}

func main() {
	targets := []string{
		"https://api.onusly.com/health",
		"https://api.onusly.com/enforcement/blocklist",
		"https://api.onusly.com/downloads/updates.xml",
	}

	for _, t := range targets {
		fmt.Printf("=== Inspecting %s ===\n", t)
		r := inspectEndpoint(t)
		fmt.Printf("  DNS Success:             %v (IPs: %v)\n", r.DNSResolutionSuccess, r.ResolvedIPs)
		fmt.Printf("  TCP Connect Success:     %v\n", r.TCPConnectSuccess)
		fmt.Printf("  TLS Handshake Success:   %v (%s, %s)\n", r.TLSHandshakeSuccess, r.NegotiatedTLSVersion, r.NegotiatedCipherSuite)
		fmt.Printf("  Cert Subject:            %s (Issuer: %s)\n", r.ServerCertSubject, r.ServerCertIssuer)
		fmt.Printf("  HTTP Response Received:  %v (Status: %d, Proto: %s)\n", r.HTTPResponseReceived, r.HTTPStatusCode, r.HTTPProtocol)
		fmt.Printf("  Cloudflare Detected:     %v (CF-RAY: %s)\n", r.CloudflareEdgeDetected, r.CloudflareRayID)
		fmt.Printf("  Cloudflare Origin Down:  %v\n", r.CloudflareOriginDown)
		fmt.Printf("  Backend Healthy:         %v\n", r.BackendHealthy)
		bodyPreview := r.ResponseBody
		if len(bodyPreview) > 80 {
			bodyPreview = bodyPreview[:80] + "..."
		}
		fmt.Printf("  Response Body:           %q\n\n", bodyPreview)
	}
}
