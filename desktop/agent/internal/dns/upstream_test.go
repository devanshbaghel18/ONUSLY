package dns

import (
	"context"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestUpstreamPool_RejectionOfHostnames(t *testing.T) {
	// Upstream pool MUST reject hostnames to prevent recursive DNS query deadlocks
	_, err := NewUpstreamPool([]string{"dns.google.com:53"}, 1*time.Second)
	if err == nil {
		t.Fatalf("Expected error when providing hostname as upstream, got nil")
	}

	// Valid IP addresses must succeed
	pool, err := NewUpstreamPool([]string{"1.1.1.1:53", "8.8.8.8"}, 1*time.Second)
	if err != nil {
		t.Fatalf("Expected success for valid IP addresses, got: %v", err)
	}
	if len(pool.servers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(pool.servers))
	}
}

func TestUpstreamPool_Failover(t *testing.T) {
	// Server 1 is a dead port (times out immediately)
	deadAddr := "127.0.0.1:59998"

	// Server 2 is a functioning mock upstream
	liveAddr := "127.0.0.1:59997"
	cleanup := startMockUpstreamServer(t, liveAddr)
	defer cleanup()

	pool, err := NewUpstreamPool([]string{deadAddr, liveAddr}, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("NewUpstreamPool error: %v", err)
	}

	qName, _ := dnsmessage.NewName("example.com.")
	queryMsg := dnsmessage.Message{
		Header: dnsmessage.Header{ID: 0x9999, RecursionDesired: true},
		Questions: []dnsmessage.Question{
			{Name: qName, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
		},
	}
	qBytes, _ := queryMsg.Pack()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	respBytes, err := pool.Exchange(ctx, qBytes)
	if err != nil {
		t.Fatalf("Expected failover to succeed, got error: %v", err)
	}

	var respMsg dnsmessage.Message
	if err := respMsg.Unpack(respBytes); err != nil {
		t.Fatalf("Failed to unpack response: %v", err)
	}
	if respMsg.RCode != dnsmessage.RCodeSuccess {
		t.Errorf("Expected RCodeSuccess, got %v", respMsg.RCode)
	}
}

