package dns

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// ServerConfig defines the listening and upstream parameters for the DNS resolver.
type ServerConfig struct {
	ListenAddress string // e.g. "127.0.0.1:53" or "127.0.0.1:5354"
	UpstreamAddrs []string
	MaxCacheSize  int
	QueryTimeout  time.Duration
}

// ServerStats tracks runtime metrics for DNS queries without logging sensitive browsing history.
type ServerStats struct {
	QueriesTotal     atomic.Uint64
	QueriesBlocked   atomic.Uint64
	QueriesForwarded atomic.Uint64
	QueriesCached    atomic.Uint64
	QueriesFailed    atomic.Uint64
}

// Server runs the concurrent local UDP and TCP DNS enforcement resolver.
type Server struct {
	cfg        ServerConfig
	policyMgr  *PolicyManager
	cache      *MemoryDNSCache
	upstreams  *UpstreamPool
	stats      ServerStats
	udpConn    *net.UDPConn
	tcpLn      net.Listener
	mu         sync.Mutex
	running    bool
	stopChan   chan struct{}
	cancelFunc context.CancelFunc
}

func NewServer(cfg ServerConfig, policyMgr *PolicyManager, cache *MemoryDNSCache, upstreams *UpstreamPool) (*Server, error) {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = "127.0.0.1:53"
	}
	if upstreams == nil {
		var err error
		upstreams, err = NewUpstreamPool(cfg.UpstreamAddrs, cfg.QueryTimeout)
		if err != nil {
			return nil, err
		}
	}
	if cache == nil {
		cache = NewMemoryDNSCache(cfg.MaxCacheSize)
	}
	if policyMgr == nil {
		policyMgr = NewPolicyManager(cache)
	}

	return &Server{
		cfg:       cfg,
		policyMgr: policyMgr,
		cache:     cache,
		upstreams: upstreams,
		stopChan:  make(chan struct{}),
	}, nil
}

// Start binds UDP and TCP listeners on the configured loopback address and begins servicing queries.
func (s *Server) Start(parentCtx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("dns server already running")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancelFunc = cancel

	// 1. Bind UDP Listener
	udpAddr, err := net.ResolveUDPAddr("udp", s.cfg.ListenAddress)
	if err != nil {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("invalid UDP listen address %s: %w", s.cfg.ListenAddress, err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("failed to bind UDP DNS listener on %s: %w", s.cfg.ListenAddress, err)
	}
	s.udpConn = udpConn

	// 2. Bind TCP Listener
	tcpLn, err := net.Listen("tcp", s.cfg.ListenAddress)
	if err != nil {
		_ = udpConn.Close()
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("failed to bind TCP DNS listener on %s: %w", s.cfg.ListenAddress, err)
	}
	s.tcpLn = tcpLn
	s.running = true
	s.mu.Unlock()

	go s.udpListenLoop(ctx, udpConn)
	go s.tcpListenLoop(ctx, tcpLn)

	return nil
}

// Stop gracefully shuts down both UDP and TCP listeners.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	close(s.stopChan)

	if s.udpConn != nil {
		_ = s.udpConn.Close()
	}
	if s.tcpLn != nil {
		_ = s.tcpLn.Close()
	}
	s.mu.Unlock()

	return nil
}

// IsRunning reports whether the server is actively listening.
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// GetStats returns current aggregate query statistics.
func (s *Server) GetStats() (total, blocked, forwarded, cached, failed uint64) {
	return s.stats.QueriesTotal.Load(),
		s.stats.QueriesBlocked.Load(),
		s.stats.QueriesForwarded.Load(),
		s.stats.QueriesCached.Load(),
		s.stats.QueriesFailed.Load()
}

// ListenAddress returns the configured listening address.
func (s *Server) ListenAddress() string {
	return s.cfg.ListenAddress
}

// GetPolicyManager returns the associated policy manager.
func (s *Server) GetPolicyManager() *PolicyManager {
	return s.policyMgr
}

// GetCache returns the in-memory DNS cache.
func (s *Server) GetCache() *MemoryDNSCache {
	return s.cache
}

// GetUpstreamPool returns the upstream resolver pool.
func (s *Server) GetUpstreamPool() *UpstreamPool {
	return s.upstreams
}

func (s *Server) udpListenLoop(ctx context.Context, conn *net.UDPConn) {
	for {
		var buf [4096]byte
		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				log.Printf("[DNS UDP] Read error: %v", err)
				continue
			}
		}

		rawQuery := make([]byte, n)
		copy(rawQuery, buf[:n])

		go func(clientAddr *net.UDPAddr, queryData []byte) {
			s.stats.QueriesTotal.Add(1)
			respData := s.handleQuery(ctx, queryData)
			if len(respData) > 0 {
				_, _ = conn.WriteToUDP(respData, clientAddr)
			}
		}(remoteAddr, rawQuery)
	}
}

func (s *Server) tcpListenLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				log.Printf("[DNS TCP] Accept error: %v", err)
				continue
			}
		}

		go s.handleTCPConnection(ctx, conn)
	}
}

func (s *Server) handleTCPConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	for {
		var lenBuf [2]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			return
		}
		queryLen := binary.BigEndian.Uint16(lenBuf[:])
		if queryLen == 0 || queryLen > 65535 {
			return
		}

		queryBuf := make([]byte, queryLen)
		if _, err := io.ReadFull(conn, queryBuf); err != nil {
			return
		}

		s.stats.QueriesTotal.Add(1)
		respData := s.handleQuery(ctx, queryBuf)
		if len(respData) == 0 {
			return
		}

		var respLenBuf [2]byte
		binary.BigEndian.PutUint16(respLenBuf[:], uint16(len(respData)))
		if _, err := conn.Write(respLenBuf[:]); err != nil {
			return
		}
		if _, err := conn.Write(respData); err != nil {
			return
		}
	}
}

func (s *Server) handleQuery(ctx context.Context, rawQuery []byte) []byte {
	var msg dnsmessage.Message
	if err := msg.Unpack(rawQuery); err != nil || len(msg.Questions) == 0 {
		s.stats.QueriesFailed.Add(1)
		return buildServFailResponse(rawQuery)
	}

	question := msg.Questions[0]
	qNameStr := question.Name.String()
	cleanDomain := NormalizeDomain(qNameStr)

	// 1. Evaluate enforcement policy against the atomic snapshot
	if isBlocked, _ := s.policyMgr.CheckBlocked(cleanDomain); isBlocked {
		s.stats.QueriesBlocked.Add(1)
		return buildNXDomainResponse(&msg, question)
	}

	// 2. Check local DNS cache for allowed domains
	if cachedBytes, found := s.cache.Get(cleanDomain, question.Type, question.Class); found {
		s.stats.QueriesCached.Add(1)
		// Restore original client Query ID
		if len(cachedBytes) >= 2 {
			cachedBytes[0] = byte(msg.ID >> 8)
			cachedBytes[1] = byte(msg.ID)
		}
		return cachedBytes
	}

	// 3. Forward to upstream resolvers
	respBytes, err := s.upstreams.Exchange(ctx, rawQuery)
	if err != nil {
		s.stats.QueriesFailed.Add(1)
		return buildServFailResponse(rawQuery)
	}

	s.stats.QueriesForwarded.Add(1)

	// 4. Cache unblocked upstream response (if valid response)
	var respMsg dnsmessage.Message
	if errUnpack := respMsg.Unpack(respBytes); errUnpack == nil {
		s.cache.Set(cleanDomain, question.Type, question.Class, respMsg.RCode, respBytes)
	}

	return respBytes
}

// buildNXDomainResponse synthesizes an authoritative NXDOMAIN response for blocked domains.
// Invariant: Never sets the AD (Authenticated Data) bit and never forges DNSSEC signatures.
func buildNXDomainResponse(query *dnsmessage.Message, question dnsmessage.Question) []byte {
	resp := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 query.ID,
			Response:           true,
			OpCode:             query.OpCode,
			Authoritative:      true,
			RecursionAvailable: true,
			RecursionDesired:   query.RecursionDesired,
			RCode:              dnsmessage.RCodeNameError, // NXDOMAIN
		},
		Questions: []dnsmessage.Question{question},
	}

	packed, err := resp.Pack()
	if err != nil {
		return buildServFailResponse(nil)
	}
	return packed
}

// buildServFailResponse returns a fallback SERVFAIL response on internal or upstream error.
func buildServFailResponse(rawQuery []byte) []byte {
	var id uint16
	if len(rawQuery) >= 2 {
		id = binary.BigEndian.Uint16(rawQuery[:2])
	}
	resp := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       id,
			Response: true,
			RCode:    dnsmessage.RCodeServerFailure,
		},
	}
	packed, _ := resp.Pack()
	return packed
}
