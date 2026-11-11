package dns_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/dns"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"golang.org/x/net/dns/dnsmessage"
)

// setupTestDNSServer creates an isolated, local in-process DNS server with mock upstreams.
func setupTestDNSServer(t *testing.T) (*dns.Server, *dns.PolicyManager, *dns.MemoryDNSCache, *mockUpstreamServer) {
	mockUpstream := newMockUpstreamServer(t)

	cache := dns.NewMemoryDNSCache(500)
	policyMgr := dns.NewPolicyManager(cache)

	pool, err := dns.NewUpstreamPool([]string{mockUpstream.Addr()}, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to create UpstreamPool: %v", err)
	}

	cfg := dns.ServerConfig{
		ListenAddress: "127.0.0.1:0", // Ephemeral loopback port
		MaxCacheSize:  500,
		QueryTimeout:  2 * time.Second,
	}

	server, err := dns.NewServer(cfg, policyMgr, cache, pool)
	if err != nil {
		t.Fatalf("Failed to create DNSServer: %v", err)
	}

	// Find free ephemeral port for UDP and TCP
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find free UDP port: %v", err)
	}
	freeAddr := pc.LocalAddr().String()
	pc.Close()

	// Reconfigure with known free ephemeral address
	serverConfig := dns.ServerConfig{
		ListenAddress: freeAddr,
		MaxCacheSize:  500,
		QueryTimeout:  2 * time.Second,
	}
	server, _ = dns.NewServer(serverConfig, policyMgr, cache, pool)

	ctx, cancel := context.WithCancel(context.Background())
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start DNSServer on %s: %v", freeAddr, err)
	}

	t.Cleanup(func() {
		cancel()
		_ = server.Stop()
		mockUpstream.Close()
	})

	return server, policyMgr, cache, mockUpstream
}

type mockUpstreamServer struct {
	udpConn *net.UDPConn
	addr    string
}

func newMockUpstreamServer(t *testing.T) *mockUpstreamServer {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr failed: %v", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}

	m := &mockUpstreamServer{
		udpConn: conn,
		addr:    conn.LocalAddr().String(),
	}

	go m.serve()
	return m
}

func (m *mockUpstreamServer) Addr() string {
	return m.addr
}

func (m *mockUpstreamServer) Close() {
	if m.udpConn != nil {
		_ = m.udpConn.Close()
	}
}

func (m *mockUpstreamServer) serve() {
	var buf [2048]byte
	for {
		n, remoteAddr, err := m.udpConn.ReadFrom(buf[:])
		if err != nil {
			return
		}

		var req dnsmessage.Message
		if err := req.Unpack(buf[:n]); err != nil {
			continue
		}

		resp := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:                  req.ID,
				Response:            true,
				OpCode:              req.OpCode,
				Authoritative:       false,
				RecursionDesired:    req.RecursionDesired,
				RecursionAvailable:  true,
				RCode:               dnsmessage.RCodeSuccess,
			},
			Questions: req.Questions,
		}

		if len(req.Questions) > 0 {
			q := req.Questions[0]
			resp.Answers = []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:  q.Name,
						Type:  dnsmessage.TypeA,
						Class: dnsmessage.ClassINET,
						TTL:   300,
					},
					Body: &dnsmessage.AResource{
						A: [4]byte{93, 184, 216, 34}, // Example IP
					},
				},
			}
		}

		packed, err := resp.Pack()
		if err == nil {
			_, _ = m.udpConn.WriteTo(packed, remoteAddr)
		}
	}
}

func queryDNS(t *testing.T, serverAddr string, domain string, qType dnsmessage.Type) (*dnsmessage.Message, error) {
	var msg dnsmessage.Message
	msg.Header.ID = 0xABCD
	msg.Header.RecursionDesired = true

	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	name, err := dnsmessage.NewName(domain)
	if err != nil {
		return nil, err
	}

	msg.Questions = []dnsmessage.Question{
		{
			Name:  name,
			Type:  qType,
			Class: dnsmessage.ClassINET,
		},
	}

	raw, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("udp", serverAddr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(raw); err != nil {
		return nil, err
	}

	var respBuf [4096]byte
	n, err := conn.Read(respBuf[:])
	if err != nil {
		return nil, err
	}

	var resp dnsmessage.Message
	if err := resp.Unpack(respBuf[:n]); err != nil {
		return nil, err
	}

	return &resp, nil
}

// 1. Goal Status Enforcement Contract Test
func TestD4_GoalStatusEnforcementContract(t *testing.T) {
	server, policyMgr, _, _ := setupTestDNSServer(t)

	// Blocklist with active, proof_submitted, rejected, and completed goals
	blocklist := &core.BlocklistData{
		UserID:  "test-user",
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID:    "goal-1",
				GoalTitle: "Active Focus Goal",
				Status:    "active",
				Domains:   []string{"distraction.com"},
			},
			{
				GoalID:    "goal-2",
				GoalTitle: "Proof Submitted Goal",
				Status:    "proof_submitted",
				Domains:   []string{"social-network.com"},
			},
			{
				GoalID:    "goal-3",
				GoalTitle: "Rejected Proof Goal",
				Status:    "rejected",
				Domains:   []string{"gambling-site.com"},
			},
			{
				GoalID:    "goal-4",
				GoalTitle: "Completed Goal",
				Status:    "completed",
				Domains:   []string{"productive-tool.com"},
			},
		},
	}

	policyMgr.UpdateFromBlocklist(blocklist)

	tests := []struct {
		domain      string
		expectBlock bool
		status      string
	}{
		{"distraction.com", true, "active"},
		{"social-network.com", true, "proof_submitted"},
		{"gambling-site.com", true, "rejected"},
		{"productive-tool.com", false, "completed"},
		{"unrelated-news.com", false, "not in blocklist"},
	}

	for _, tc := range tests {
		resp, err := queryDNS(t, server.ListenAddress(), tc.domain, dnsmessage.TypeA)
		if err != nil {
			t.Fatalf("Query for %s failed: %v", tc.domain, err)
		}

		if tc.expectBlock {
			if resp.Header.RCode != dnsmessage.RCodeNameError {
				t.Errorf("[%s] Expected NXDOMAIN (RCodeNameError) for %s, got %v", tc.status, tc.domain, resp.Header.RCode)
			}
			if !resp.Header.Authoritative {
				t.Errorf("[%s] Expected Authoritative flag on blocked response", tc.status)
			}
			if resp.Header.AuthenticData {
				t.Errorf("[%s] AuthenticData MUST be false on synthesized NXDOMAIN (DNSSEC invariant)", tc.status)
			}
		} else {
			if resp.Header.RCode != dnsmessage.RCodeSuccess {
				t.Errorf("[%s] Expected RCodeSuccess for %s, got %v", tc.status, tc.domain, resp.Header.RCode)
			}
		}
	}

	// Dynamic Unblock Test: Goal 1 transitions from "active" -> "completed"
	blocklist.Targets[0].Status = "completed"
	blocklist.Version = "v2"
	policyMgr.UpdateFromBlocklist(blocklist)

	// Now distraction.com must be immediately unblocked without restarting the resolver!
	respAfter, err := queryDNS(t, server.ListenAddress(), "distraction.com", dnsmessage.TypeA)
	if err != nil {
		t.Fatalf("Query after unblock failed: %v", err)
	}
	if respAfter.Header.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected distraction.com to be unblocked (RCodeSuccess) after status=completed, got %v", respAfter.Header.RCode)
	}
}

// 2. Subdomains, WWW parity, and IDN Punycode Matching
func TestD4_SubdomainAndDomainMatchingRules(t *testing.T) {
	server, policyMgr, _, _ := setupTestDNSServer(t)

	blocklist := &core.BlocklistData{
		UserID:  "test-user",
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID:  "goal-sub",
				Status:  "active",
				Domains: []string{"example.com", "münchen.de"},
			},
		},
	}
	policyMgr.UpdateFromBlocklist(blocklist)

	matchTests := []struct {
		domain      string
		expectBlock bool
		reason      string
	}{
		{"example.com", true, "exact match"},
		{"www.example.com", true, "www parity"},
		{"api.example.com", true, "subdomain match"},
		{"deep.nested.sub.example.com", true, "nested subdomain match"},
		{"notexample.com", false, "boundary check prevents substring false positive"},
		{"example.com.org", false, "boundary check prevents prefix false positive"},
		{"xn--mnchen-3ya.de", true, "punycode representation of münchen.de"},
	}

	for _, mt := range matchTests {
		resp, err := queryDNS(t, server.ListenAddress(), mt.domain, dnsmessage.TypeA)
		if err != nil {
			t.Fatalf("Query for %s (%s) failed: %v", mt.domain, mt.reason, err)
		}
		isBlocked := resp.Header.RCode == dnsmessage.RCodeNameError
		if isBlocked != mt.expectBlock {
			t.Errorf("Match rule failed for %s (%s): expected blocked=%t, got blocked=%t",
				mt.domain, mt.reason, mt.expectBlock, isBlocked)
		}
	}
}

// 3. Common Record Types Blocked
func TestD4_RecordTypesBlocked(t *testing.T) {
	server, policyMgr, _, _ := setupTestDNSServer(t)

	blocklist := &core.BlocklistData{
		UserID:  "test-user",
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID:  "goal-types",
				Status:  "active",
				Domains: []string{"blocked-types.com"},
			},
		},
	}
	policyMgr.UpdateFromBlocklist(blocklist)

	recordTypes := []dnsmessage.Type{
		dnsmessage.TypeA,
		dnsmessage.TypeAAAA,
		dnsmessage.TypeCNAME,
		dnsmessage.TypeTXT,
		dnsmessage.TypeMX,
		dnsmessage.TypeSRV,
		dnsmessage.TypePTR,
	}

	for _, rt := range recordTypes {
		resp, err := queryDNS(t, server.ListenAddress(), "blocked-types.com", rt)
		if err != nil {
			t.Fatalf("Query for type %v failed: %v", rt, err)
		}
		if resp.Header.RCode != dnsmessage.RCodeNameError {
			t.Errorf("Record type %v was not blocked with NXDOMAIN, got %v", rt, resp.Header.RCode)
		}
		if len(resp.Questions) == 0 || resp.Questions[0].Type != rt {
			t.Errorf("Record type %v question was not preserved in response", rt)
		}
	}
}

// 4. Protected Domains Immunity
func TestD4_ProtectedDomainsImmunity(t *testing.T) {
	server, policyMgr, _, _ := setupTestDNSServer(t)

	// Attempt malicious or accidental blocklist entry targeting core infrastructure
	blocklist := &core.BlocklistData{
		UserID:  "test-user",
		Version: "v1",
		Targets: []core.TargetItem{
			{
				GoalID: "goal-infra",
				Status: "active",
				Domains: []string{
					"localhost",
					"127.0.0.1",
					"::1",
					"api.onusly.com",
					"static.onusly.com",
					"mycomputer.local",
					"router.internal",
					"router.home.arpa",
					"1.168.192.in-addr.arpa",
				},
			},
		},
	}
	policyMgr.UpdateFromBlocklist(blocklist)

	for _, d := range blocklist.Targets[0].Domains {
		resp, err := queryDNS(t, server.ListenAddress(), d, dnsmessage.TypeA)
		if err != nil {
			t.Fatalf("Query for protected domain %s failed: %v", d, err)
		}
		if resp.Header.RCode == dnsmessage.RCodeNameError {
			t.Errorf("SECURITY INVARIANT VIOLATION: Protected domain %s was blocked!", d)
		}
	}
}

// 5. Zero-Query Logging Privacy Invariant
func TestD4_ZeroQueryLoggingPrivacy(t *testing.T) {
	server, policyMgr, _, _ := setupTestDNSServer(t)

	blocklist := &core.BlocklistData{
		UserID:  "test-user",
		Version: "v1",
		Targets: []core.TargetItem{
			{GoalID: "g1", Status: "active", Domains: []string{"bad.com"}},
		},
	}
	policyMgr.UpdateFromBlocklist(blocklist)

	_, _ = queryDNS(t, server.ListenAddress(), "bad.com", dnsmessage.TypeA)
	_, _ = queryDNS(t, server.ListenAddress(), "allowed1.com", dnsmessage.TypeA)
	_, _ = queryDNS(t, server.ListenAddress(), "allowed2.com", dnsmessage.TypeA)

	tot, blk, fwd, cch, fld := server.GetStats()
	if tot != 3 {
		t.Errorf("Expected 3 total queries, got %d", tot)
	}
	if blk != 1 {
		t.Errorf("Expected 1 blocked query, got %d", blk)
	}
	if fwd+cch != 2 {
		t.Errorf("Expected 2 forwarded/cached queries, got fwd=%d cch=%d", fwd, cch)
	}
	if fld != 0 {
		t.Errorf("Expected 0 failed queries, got %d", fld)
	}
}

