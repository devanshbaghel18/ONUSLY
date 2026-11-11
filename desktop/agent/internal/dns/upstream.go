package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

var (
	ErrNoUpstreamsAvailable = errors.New("no upstream DNS resolvers available")
	ErrUpstreamTimeout      = errors.New("upstream DNS query timed out")
	ErrTruncatedResponse    = errors.New("DNS response truncated")
)

// UpstreamServer represents a remote DNS server configuration and runtime health status.
type UpstreamServer struct {
	Address         string // Must be an IP:port (e.g., "1.1.1.1:53"), never a hostname
	LastSuccessAt   time.Time
	LastFailureAt   time.Time
	ConsecutiveFails int
	IsHealthy       bool
}

// UpstreamPool manages multiple upstream DNS servers with health tracking, failover, and TCP fallback.
type UpstreamPool struct {
	mu           sync.RWMutex
	servers      []*UpstreamServer
	queryTimeout time.Duration
}

// NewUpstreamPool initializes an upstream resolver pool.
func NewUpstreamPool(addresses []string, timeout time.Duration) (*UpstreamPool, error) {
	if len(addresses) == 0 {
		// Default public fallbacks if none provided
		addresses = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
	}

	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}

	servers := make([]*UpstreamServer, 0, len(addresses))
	for _, addr := range addresses {
		// Ensure host has port
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = "53"
		}
		// Strict Invariant: Upstream addresses MUST be valid IP addresses to prevent recursive resolution loops
		ip := net.ParseIP(stringsTrimBrackets(host))
		if ip == nil {
			return nil, fmt.Errorf("upstream address %q is not a valid IP address: upstream hostnames are forbidden to prevent recursive loops", addr)
		}

		cleanAddr := net.JoinHostPort(ip.String(), port)
		servers = append(servers, &UpstreamServer{
			Address:   cleanAddr,
			IsHealthy: true,
		})
	}

	return &UpstreamPool{
		servers:      servers,
		queryTimeout: timeout,
	}, nil
}

func stringsTrimBrackets(s string) string {
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}

// Exchange forwards a DNS query wire payload to upstreams with automatic failover and TCP fallback.
func (p *UpstreamPool) Exchange(ctx context.Context, queryBytes []byte) ([]byte, error) {
	p.mu.RLock()
	servers := make([]*UpstreamServer, len(p.servers))
	copy(servers, p.servers)
	p.mu.RUnlock()

	if len(servers) == 0 {
		return nil, ErrNoUpstreamsAvailable
	}

	var lastErr error

	for _, srv := range servers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		respBytes, err := p.queryServer(ctx, srv.Address, queryBytes)
		if err == nil && len(respBytes) > 0 {
			p.markSuccess(srv.Address)
			return respBytes, nil
		}

		p.markFailure(srv.Address)
		lastErr = err
	}

	return nil, fmt.Errorf("all upstream DNS resolvers failed: %w", lastErr)
}

func (p *UpstreamPool) queryServer(ctx context.Context, addr string, queryBytes []byte) ([]byte, error) {
	// 1. Attempt UDP first
	udpResp, isTruncated, err := p.queryUDP(ctx, addr, queryBytes)
	if err == nil {
		if !isTruncated {
			return udpResp, nil
		}
		// If UDP response was truncated (TC bit set), perform TCP fallback
	}

	// 2. TCP fallback (e.g. for large responses, DNSSEC records, or when UDP failed with truncation)
	return p.queryTCP(ctx, addr, queryBytes)
}

func (p *UpstreamPool) queryUDP(ctx context.Context, addr string, queryBytes []byte) ([]byte, bool, error) {
	dialer := net.Dialer{Timeout: p.queryTimeout}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(p.queryTimeout))

	if _, err := conn.Write(queryBytes); err != nil {
		return nil, false, err
	}

	var buf [4096]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		return nil, false, err
	}

	// Check if truncated
	var parser dnsmessage.Parser
	hdr, err := parser.Start(buf[:n])
	if err == nil && hdr.Truncated {
		return buf[:n], true, nil
	}

	return buf[:n], false, nil
}

func (p *UpstreamPool) queryTCP(ctx context.Context, addr string, queryBytes []byte) ([]byte, error) {
	dialer := net.Dialer{Timeout: p.queryTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(p.queryTimeout))

	// Prefix with 2-byte length for TCP DNS framing (RFC 1035 §4.2.2)
	var lenPrefix [2]byte
	binary.BigEndian.PutUint16(lenPrefix[:], uint16(len(queryBytes)))

	if _, err := conn.Write(lenPrefix[:]); err != nil {
		return nil, err
	}
	if _, err := conn.Write(queryBytes); err != nil {
		return nil, err
	}

	// Read 2-byte response length
	if _, err := io.ReadFull(conn, lenPrefix[:]); err != nil {
		return nil, err
	}
	respLen := binary.BigEndian.Uint16(lenPrefix[:])
	if respLen == 0 || respLen > 65535 {
		return nil, errors.New("invalid TCP DNS response length")
	}

	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, err
	}

	return respBuf, nil
}

func (p *UpstreamPool) markSuccess(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.servers {
		if s.Address == addr {
			s.LastSuccessAt = time.Now()
			s.ConsecutiveFails = 0
			s.IsHealthy = true
			break
		}
	}
}

func (p *UpstreamPool) markFailure(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.servers {
		if s.Address == addr {
			s.LastFailureAt = time.Now()
			s.ConsecutiveFails++
			if s.ConsecutiveFails >= 3 {
				s.IsHealthy = false
			}
			break
		}
	}
}

// GetHealth returns the health summary of the upstream resolver pool.
func (p *UpstreamPool) GetHealth() (healthyCount int, totalCount int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.servers {
		if s.IsHealthy {
			healthyCount++
		}
	}
	return healthyCount, len(p.servers)
}

