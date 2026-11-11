package dns

import (
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

type cacheEntry struct {
	responseBytes []byte
	questionName  string
	qType         dnsmessage.Type
	qClass        dnsmessage.Class
	rCode         dnsmessage.RCode
	expiresAt     time.Time
	initialTTL    uint32
}

// MemoryDNSCache is a thread-safe, bounded, in-memory cache for DNS responses.
// Enforces maximum entry bounds to protect against memory exhaustion,
// tracks remaining TTLs dynamically, and supports instantaneous invalidation
// when enforcement policy changes.
type MemoryDNSCache struct {
	mu         sync.RWMutex
	entries    map[string]*cacheEntry
	maxEntries int
}

func NewMemoryDNSCache(maxEntries int) *MemoryDNSCache {
	if maxEntries <= 0 {
		maxEntries = 5000 // Default to 5000 records (~5-10 MB max memory)
	}
	return &MemoryDNSCache{
		entries:    make(map[string]*cacheEntry),
		maxEntries: maxEntries,
	}
}

func makeCacheKey(name string, qtype dnsmessage.Type, qclass dnsmessage.Class) string {
	return NormalizeDomain(name) + "|" + qtype.String() + "|" + qclass.String()
}

// Get returns the cached DNS response wire bytes if present and unexpired.
// Dynamically adjusts the resource record TTLs to reflect elapsed time since insertion.
func (c *MemoryDNSCache) Get(name string, qtype dnsmessage.Type, qclass dnsmessage.Class) ([]byte, bool) {
	key := makeCacheKey(name, qtype, qclass)

	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()

	if !found {
		return nil, false
	}

	now := time.Now()
	if now.After(entry.expiresAt) {
		// Lazily evict expired entry
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	remaining := uint32(entry.expiresAt.Sub(now).Seconds())
	if remaining == 0 {
		remaining = 1
	}

	// Unpack and adjust TTL in the wire format
	var msg dnsmessage.Message
	if err := msg.Unpack(entry.responseBytes); err != nil {
		return nil, false
	}

	for i := range msg.Answers {
		msg.Answers[i].Header.TTL = remaining
	}
	for i := range msg.Authorities {
		msg.Authorities[i].Header.TTL = remaining
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, false
	}

	return packed, true
}

// Set stores a DNS response in cache, extracting the minimum TTL from the records.
func (c *MemoryDNSCache) Set(name string, qtype dnsmessage.Type, qclass dnsmessage.Class, rCode dnsmessage.RCode, rawBytes []byte) {
	var msg dnsmessage.Message
	if err := msg.Unpack(rawBytes); err != nil {
		return
	}

	minTTL := uint32(300) // Default 5 minutes
	hasRecords := false

	for _, a := range msg.Answers {
		if !hasRecords || a.Header.TTL < minTTL {
			minTTL = a.Header.TTL
			hasRecords = true
		}
	}

	// For NXDOMAIN or empty responses, use SOA TTL or default negative TTL
	if !hasRecords {
		for _, auth := range msg.Authorities {
			if !hasRecords || auth.Header.TTL < minTTL {
				minTTL = auth.Header.TTL
				hasRecords = true
			}
		}
	}

	// Clamp TTL between 1 second and 86400 seconds (24 hours)
	if minTTL < 1 {
		minTTL = 1
	}
	if minTTL > 86400 {
		minTTL = 86400
	}

	key := makeCacheKey(name, qtype, qclass)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Enforce capacity bound: evict older entries if capacity exceeded
	if len(c.entries) >= c.maxEntries {
		// Evict first found expired entry or random entry
		now := time.Now()
		evicted := false
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
				evicted = true
				break
			}
		}
		if !evicted {
			// If none expired, evict one arbitrary entry
			for k := range c.entries {
				delete(c.entries, k)
				break
			}
		}
	}

	c.entries[key] = &cacheEntry{
		responseBytes: rawBytes,
		questionName:  name,
		qType:         qtype,
		qClass:        qclass,
		rCode:         rCode,
		expiresAt:     time.Now().Add(time.Duration(minTTL) * time.Second),
		initialTTL:    minTTL,
	}
}

// Invalidate purges all cached entries that match any of the newly blocked domains.
// Ensures that policy updates take immediate effect without waiting for DNS TTL expiration.
func (c *MemoryDNSCache) Invalidate(targetDomains []string) {
	if len(targetDomains) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, entry := range c.entries {
		for _, target := range targetDomains {
			if IsDomainMatch(entry.questionName, target) {
				delete(c.entries, k)
				break
			}
		}
	}
}

// Clear flushes all entries from the cache.
func (c *MemoryDNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// Count returns the number of active entries currently in cache.
func (c *MemoryDNSCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
