package dns

import (
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDNSCache_TTLAndExpiration(t *testing.T) {
	cache := NewMemoryDNSCache(10)

	qName := "example.com."
	qType := dnsmessage.TypeA
	qClass := dnsmessage.ClassINET

	rawResp := buildMockResponseRecord(qName, 2) // 2 second initial TTL

	cache.Set(qName, qType, qClass, dnsmessage.RCodeSuccess, rawResp)

	// Immediate hit
	cached1, found := cache.Get(qName, qType, qClass)
	if !found || len(cached1) == 0 {
		t.Fatalf("Expected cache hit immediately after Set")
	}

	// Verify TTL is preserved or adjusted
	var m1 dnsmessage.Message
	_ = m1.Unpack(cached1)
	if len(m1.Answers) == 0 || m1.Answers[0].Header.TTL == 0 {
		t.Errorf("Expected valid non-zero TTL in cached response")
	}

	// Wait 2.2 seconds for expiration
	time.Sleep(2200 * time.Millisecond)

	// Should be expired
	_, foundAfterExp := cache.Get(qName, qType, qClass)
	if foundAfterExp {
		t.Errorf("Expected entry to be expired after TTL duration")
	}
}

func TestDNSCache_CapacityEviction(t *testing.T) {
	cache := NewMemoryDNSCache(3) // Cap at 3 items

	for i := 1; i <= 5; i++ {
		name := string(rune('a'+i)) + ".com."
		raw := buildMockResponseRecord(name, 100)
		cache.Set(name, dnsmessage.TypeA, dnsmessage.ClassINET, dnsmessage.RCodeSuccess, raw)
	}

	if cache.Count() > 3 {
		t.Errorf("Expected cache size bounded at <= 3, got %d", cache.Count())
	}
}

func TestDNSCache_InvalidateTargetDomains(t *testing.T) {
	cache := NewMemoryDNSCache(50)

	cache.Set("facebook.com.", dnsmessage.TypeA, dnsmessage.ClassINET, dnsmessage.RCodeSuccess, buildMockResponseRecord("facebook.com.", 60))
	cache.Set("m.facebook.com.", dnsmessage.TypeA, dnsmessage.ClassINET, dnsmessage.RCodeSuccess, buildMockResponseRecord("m.facebook.com.", 60))
	cache.Set("google.com.", dnsmessage.TypeA, dnsmessage.ClassINET, dnsmessage.RCodeSuccess, buildMockResponseRecord("google.com.", 60))

	if cache.Count() != 3 {
		t.Fatalf("Expected 3 items in cache, got %d", cache.Count())
	}

	// Invalidate "facebook.com" (should purge facebook.com and m.facebook.com, but keep google.com)
	cache.Invalidate([]string{"facebook.com"})

	_, fbFound := cache.Get("facebook.com.", dnsmessage.TypeA, dnsmessage.ClassINET)
	if fbFound {
		t.Errorf("facebook.com should have been invalidated")
	}

	_, mFbFound := cache.Get("m.facebook.com.", dnsmessage.TypeA, dnsmessage.ClassINET)
	if mFbFound {
		t.Errorf("m.facebook.com should have been invalidated")
	}

	_, gFound := cache.Get("google.com.", dnsmessage.TypeA, dnsmessage.ClassINET)
	if !gFound {
		t.Errorf("google.com should remain in cache")
	}
}

func buildMockResponseRecord(name string, ttl uint32) []byte {
	n, _ := dnsmessage.NewName(name)
	m := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 1,
			Response:           true,
			RecursionAvailable: true,
			RCode:              dnsmessage.RCodeSuccess,
		},
		Questions: []dnsmessage.Question{
			{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET},
		},
		Answers: []dnsmessage.Resource{
			{
				Header: dnsmessage.ResourceHeader{
					Name:  n,
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
					TTL:   ttl,
				},
				Body: &dnsmessage.AResource{A: [4]byte{1, 2, 3, 4}},
			},
		},
	}
	p, _ := m.Pack()
	return p
}

