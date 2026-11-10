package gobgp

import "time"

// Config is a GoBGP source config
type Config struct {
	ID   string
	Name string

	Host     string `ini:"host"`
	Insecure bool   `ini:"insecure"`
	// ProcessingTimeout is a timeout in seconds configured per gRPC call to a given GoBGP daemon
	ProcessingTimeout int    `ini:"processing_timeout"`
	TLSCert           string `ini:"tls_crt"`
	TLSCommonName     string `ini:"tls_common_name"`

	// CacheTTL is the time the routes of a neighbor are
	// kept in memory (cache_ttl in seconds, 0 disables the cache).
	CacheTTL time.Duration
	// RoutesCacheSize is the max number of cached neighbors.
	RoutesCacheSize int
}
