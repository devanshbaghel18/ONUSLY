package dns

// AdapterDNSConfig holds the saved DNS configuration for a single network interface.
type AdapterDNSConfig struct {
	InterfaceID      string   `json:"interfaceId"`
	InterfaceAlias   string   `json:"interfaceAlias"`
	InterfaceIndex   int      `json:"interfaceIndex"`
	AddressFamily    int      `json:"addressFamily"` // 2 = IPv4, 23 = IPv6
	IsDHCP           bool     `json:"isDhcp"`
	ServerAddresses  []string `json:"serverAddresses"`
	IsPhysical       bool     `json:"isPhysical"`
	Status           string   `json:"status"` // "Up", "Connected", etc.
}

// DNSBackupState represents the persisted snapshot of original adapter DNS configurations
// used for deterministic, fail-open rollback and restoration.
type DNSBackupState struct {
	Timestamp      string             `json:"timestamp"`
	Platform       string             `json:"platform"`
	OriginalDns    []AdapterDNSConfig `json:"originalDns"`
	LoopbackIPv4   string             `json:"loopbackIpv4"`
	LoopbackIPv6   string             `json:"loopbackIpv6"`
	IsActive       bool               `json:"isActive"`
}

// DNSHealthStatus summarizes the operational state of the DNS resolver and adapter redirection.
type DNSHealthStatus struct {
	ResolverRunning    bool   `json:"resolverRunning"`
	ResolverListening  bool   `json:"resolverListening"`
	InterceptionActive bool   `json:"interceptionActive"`
	UpstreamHealthy    bool   `json:"upstreamHealthy"`
	UpstreamCount      int    `json:"upstreamCount"`
	UpstreamHealthyCount int  `json:"upstreamHealthyCount"`
	QueriesTotal       uint64 `json:"queriesTotal"`
	QueriesBlocked     uint64 `json:"queriesBlocked"`
	QueriesForwarded   uint64 `json:"queriesForwarded"`
	QueriesCached      uint64 `json:"queriesCached"`
	QueriesFailed      uint64 `json:"queriesFailed"`
	LastError          string `json:"lastError,omitempty"`
}

// AdapterManager abstracts platform-specific network adapter DNS reconfiguration and rollback.
type AdapterManager interface {
	DiscoverAdapters() ([]AdapterDNSConfig, error)
	BackupDNS(backupFilePath string) (*DNSBackupState, error)
	ApplyLoopbackDNS(loopbackIPv4, loopbackIPv6 string, backupFilePath string) error
	RestoreDNS(backupFilePath string) error
	IsRedirectionActive(backupFilePath string) (bool, error)
}

