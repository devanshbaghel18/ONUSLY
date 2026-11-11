package dns

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"golang.org/x/net/dns/dnsmessage"
)

// startMockUpstreamServer starts a loopback mock DNS server responding to A records.
func startMockUpstreamServer(t *testing.T, listenAddr string) (cleanup func()) {
	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		t.Fatalf("Failed to resolve mock upstream address: %v", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("Failed to bind mock upstream UDP listener: %v", err)
	}

	tcpLn, err := net.Listen("tcp", listenAddr)
	if err != nil {
		_ = udpConn.Close()
		t.Fatalf("Failed to bind mock upstream TCP listener: %v", err)
	}

	stopChan := make(chan struct{})

	// UDP handler
	go func() {
		var buf [4096]byte
		for {
			n, remote, err := udpConn.ReadFromUDP(buf[:])
			if err != nil {
				return
			}
			resp := buildMockResponse(buf[:n])
			_, _ = udpConn.WriteToUDP(resp, remote)
		}
	}()

	// TCP handler
	go func() {
		for {
			conn, err := tcpLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var lenBuf [2]byte
				if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
					return
				}
				qLen := binary.BigEndian.Uint16(lenBuf[:])
				qBuf := make([]byte, qLen)
				if _, err := io.ReadFull(c, qBuf); err != nil {
					return
				}

				resp := buildMockResponse(qBuf)
				var rLenBuf [2]byte
				binary.BigEndian.PutUint16(rLenBuf[:], uint16(len(resp)))
				_, _ = c.Write(rLenBuf[:])
				_, _ = c.Write(resp)
			}(conn)
		}
	}()

	return func() {
		close(stopChan)
		_ = udpConn.Close()
		_ = tcpLn.Close()
	}
}

func buildMockResponse(queryBytes []byte) []byte {
	var q dnsmessage.Message
	if err := q.Unpack(queryBytes); err != nil || len(q.Questions) == 0 {
		return nil
	}

	question := q.Questions[0]
	resp := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 q.ID,
			Response:           true,
			Authoritative:      false,
			RecursionAvailable: true,
			RCode:              dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{question},
		Answers: []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  question.Name,
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
					TTL:   120,
				},
				Body: &dnsmessage.AResource{
					A: [4]byte{93, 184, 216, 34}, // Example IP: 93.184.216.34
				},
			},
		},
	}

	packed, _ := resp.Pack()
	return packed
}

func TestDNSServer_FullLifecycleAndEnforcement(t *testing.T) {
	upstreamPort := "127.0.0.1:53531"
	cleanupUpstream := startMockUpstreamServer(t, upstreamPort)
	defer cleanupUpstream()

	resolverAddr := "127.0.0.1:53532"
	cfg := ServerConfig{
		ListenAddress: resolverAddr,
		UpstreamAddrs: []string{upstreamPort},
		MaxCacheSize:  100,
		QueryTimeout:  1 * time.Second,
	}

	dnsCache := NewMemoryDNSCache(100)
	policyMgr := NewPolicyManager(dnsCache)
	upstreams, err := NewUpstreamPool(cfg.UpstreamAddrs, cfg.QueryTimeout)
	if err != nil {
		t.Fatalf("NewUpstreamPool error: %v", err)
	}

	srv, err := NewServer(cfg, policyMgr, dnsCache, upstreams)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Failed to start DNS server: %v", err)
	}
	defer srv.Stop()

	time.Sleep(50 * time.Millisecond)

	// 1. Configure initial policy snapshot with D4 goal status requirements:
	// - active: Block
	// - proof_submitted: Block
	// - rejected: Block
	// - completed: Allowed / Unblock
	initialPolicy := &core.BlocklistData{
		Version:     "v-dns-1",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Targets: []core.TargetItem{
			{
				GoalID:    "g-active",
				GoalTitle: "Block Social",
				Status:    "active",
				Domains:   []string{"facebook.com", "instagram.com"},
			},
			{
				GoalID:    "g-proof-sub",
				GoalTitle: "Block Gaming",
				Status:    "proof_submitted",
				Domains:   []string{"steamcommunity.com"},
			},
			{
				GoalID:    "g-rejected",
				GoalTitle: "Block Distractions",
				Status:    "rejected",
				Domains:   []string{"tiktok.com"},
			},
			{
				GoalID:    "g-completed",
				GoalTitle: "Finished Goal",
				Status:    "completed",
				Domains:   []string{"learning-site.org"},
			},
		},
	}

	if _, err := policyMgr.UpdateFromBlocklist(initialPolicy); err != nil {
		t.Fatalf("UpdateFromBlocklist failed: %v", err)
	}

	// 2. Query Blocked Domains (active, proof_submitted, rejected)
	blockedQueries := []string{
		"facebook.com.",
		"www.facebook.com.",
		"m.instagram.com.",
		"steamcommunity.com.",
		"tiktok.com.",
		"api.tiktok.com.",
	}

	for _, domain := range blockedQueries {
		resp, err := sendDNSQueryUDP(resolverAddr, domain, dnsmessage.TypeA)
		if err != nil {
			t.Fatalf("UDP query failed for %s: %v", domain, err)
		}
		if resp.RCode != dnsmessage.RCodeNameError {
			t.Errorf("Expected NXDOMAIN (RCodeNameError) for blocked %s, got %v", domain, resp.RCode)
		}
		if len(resp.Answers) != 0 {
			t.Errorf("Expected 0 answers for blocked %s, got %d", domain, len(resp.Answers))
		}
		if !resp.Authoritative {
			t.Errorf("Expected Authoritative=true on synthesized response for %s", domain)
		}
		if resp.AuthenticData {
			t.Errorf("CRITICAL SECURITY VIOLATION: AuthenticData (AD) bit must NEVER be set on synthesized response for %s", domain)
		}
	}

	// 3. Query Allowed Domain (completed goal: learning-site.org)
	respCompleted, err := sendDNSQueryUDP(resolverAddr, "learning-site.org.", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("Query for completed goal domain failed: %v", err)
	}
	if respCompleted.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected NOERROR (RCodeSuccess) for completed goal domain, got %v", respCompleted.RCode)
	}
	if len(respCompleted.Answers) == 0 {
		t.Errorf("Expected answers from upstream for completed goal domain")
	}

	// 4. Query Unrelated Allowed Domain (wikipedia.org)
	respAllowed, err := sendDNSQueryUDP(resolverAddr, "wikipedia.org.", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("Query for allowed domain failed: %v", err)
	}
	if respAllowed.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected NOERROR for wikipedia.org, got %v", respAllowed.RCode)
	}

	// Verify Cache Hit on subsequent query
	respCached, err := sendDNSQueryUDP(resolverAddr, "wikipedia.org.", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("Second query for wikipedia.org failed: %v", err)
	}
	if respCached.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected cached success for wikipedia.org")
	}

	_, _, _, cachedCount, _ := srv.GetStats()
	if cachedCount == 0 {
		t.Errorf("Expected cachedCount > 0, got %d", cachedCount)
	}

	// 5. Test Cache Invalidation when a domain becomes blocked
	// Before update: wikipedia.org is in cache
	updatePolicy := &core.BlocklistData{
		Version:     "v-dns-2",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Targets: []core.TargetItem{
			{
				GoalID:    "g-new-block",
				GoalTitle: "Now Blocking Wikipedia",
				Status:    "active",
				Domains:   []string{"wikipedia.org"},
			},
		},
	}
	_, _ = policyMgr.UpdateFromBlocklist(updatePolicy)

	// Now querying wikipedia.org MUST be immediately blocked (NXDOMAIN), NOT served from stale cache!
	respNowBlocked, err := sendDNSQueryUDP(resolverAddr, "wikipedia.org.", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if respNowBlocked.RCode != dnsmessage.RCodeNameError {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: Stale cache allowed domain that is now blocked!")
	}

	// 6. Test TCP Query Support
	respTCP, err := sendDNSQueryTCP(resolverAddr, "learning-site.org.", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("TCP DNS query failed: %v", err)
	}
	if respTCP.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected NOERROR on TCP query, got %v", respTCP.RCode)
	}

	// 7. Test Stats Observability (verifying aggregate counters without logging queries)
	total, blocked, forwarded, _, _ := srv.GetStats()
	if total == 0 || blocked == 0 || forwarded == 0 {
		t.Errorf("Expected non-zero stats: total=%d, blocked=%d, forwarded=%d", total, blocked, forwarded)
	}
}

func sendDNSQueryUDP(serverAddr, domain string, qType dnsmessage.Type) (*dnsmessage.Message, error) {
	conn, err := net.DialTimeout("udp", serverAddr, 1*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))

	qName, err := dnsmessage.NewName(domain)
	if err != nil {
		return nil, err
	}

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:               0x1234,
			RecursionDesired: true,
		},
		Questions: []dnsmessage.Question{
			{
				Name:  qName,
				Type:  qType,
				Class: dnsmessage.ClassINET,
			},
		},
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	if _, err := conn.Write(packed); err != nil {
		return nil, err
	}

	var buf [4096]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		return nil, err
	}

	var resp dnsmessage.Message
	if err := resp.Unpack(buf[:n]); err != nil {
		return nil, err
	}

	return &resp, nil
}

func sendDNSQueryTCP(serverAddr, domain string, qType dnsmessage.Type) (*dnsmessage.Message, error) {
	conn, err := net.DialTimeout("tcp", serverAddr, 1*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))

	qName, err := dnsmessage.NewName(domain)
	if err != nil {
		return nil, err
	}

	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:               0x5678,
			RecursionDesired: true,
		},
		Questions: []dnsmessage.Question{
			{
				Name:  qName,
				Type:  qType,
				Class: dnsmessage.ClassINET,
			},
		},
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packed)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return nil, err
	}
	if _, err := conn.Write(packed); err != nil {
		return nil, err
	}

	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, err
	}
	rLen := binary.BigEndian.Uint16(lenBuf[:])
	rBuf := make([]byte, rLen)
	if _, err := io.ReadFull(conn, rBuf); err != nil {
		return nil, err
	}

	var resp dnsmessage.Message
	if err := resp.Unpack(rBuf); err != nil {
		return nil, err
	}

	return &resp, nil
}
