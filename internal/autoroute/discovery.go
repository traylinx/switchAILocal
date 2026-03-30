package autoroute

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Supported provider connection types
const (
	AuthTypeOAuth   = "oauth"
	AuthTypeAPIKey  = "api-key"
	AuthTypeLocal   = "local"
	AuthTypeUnknown = "unknown"
)

// DiscoveredModel represents a model found during probing
type DiscoveredModel struct {
	ID      string
	Context int
}

// SubscriptionInfo holds tier details detected from probes
type SubscriptionInfo struct {
	Tier           string // "free", "standard", "premium", "unknown"
	QuotaTotal     int64  // -1 = unlimited
	QuotaRemaining int64
	ResetTime      time.Time
	Source         string // "api-header", "cli-command", "inferred", "config-override"
}

// RateLimits holds quota limits parsed from HTTP headers
type RateLimits struct {
	RequestsPerMinute int
	TokensPerMinute   int
}

// ProbeResult holds the outcome of questioning a specific provider
type ProbeResult struct {
	Provider         string
	Available        bool
	AuthType         string
	SubscriptionInfo *SubscriptionInfo
	Models           []DiscoveredModel
	RateLimits       *RateLimits
	Latency          time.Duration
	ProbeError       error
	ProbedAt         time.Time
}

// ProviderProber defines the interface for probing a specific backend
type ProviderProber interface {
	// Name returns the provider name (e.g., "geminicli")
	Name() string
	// Probe executes the health/discovery check
	Probe(ctx context.Context) ProbeResult
}

// DiscoveryService orchestrates parallel health and tier probing
type DiscoveryService struct {
	config   DiscoveryConfig
	probers  []ProviderProber
	cache    map[string]ProbeResult
	cacheMu  sync.RWMutex
	lastRun  time.Time
}

// NewDiscoveryService initializes the orchestrator with configured probers
func NewDiscoveryService(cfg DiscoveryConfig) *DiscoveryService {
	return &DiscoveryService{
		config:  cfg,
		probers: make([]ProviderProber, 0),
		cache:   make(map[string]ProbeResult),
	}
}

// RegisterProber adds a provider-specific probe to the pipeline
func (s *DiscoveryService) RegisterProber(p ProviderProber) {
	s.probers = append(s.probers, p)
}

// DiscoverAll runs all registered probes in parallel
func (s *DiscoveryService) DiscoverAll(ctx context.Context) map[string]ProbeResult {
	if !s.config.Enabled {
		log.Debug("auto-discovery is disabled in config")
		return nil
	}

	// Respect cache TTL
	s.cacheMu.RLock()
	if !s.lastRun.IsZero() && time.Since(s.lastRun) < s.config.CacheTTL {
		defer s.cacheMu.RUnlock()
		return s.cloneCache()
	}
	s.cacheMu.RUnlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, s.config.ProbeTimeout)
	defer cancel()

	results := make(chan ProbeResult, len(s.probers))
	var wg sync.WaitGroup

	log.WithField("probers", len(s.probers)).Debug("starting parallel provider auto-discovery")
	start := time.Now()

	for _, p := range s.probers {
		wg.Add(1)
		go func(prober ProviderProber) {
			defer wg.Done()
			res := prober.Probe(timeoutCtx)
			res.ProbedAt = time.Now()
			results <- res
		}(p)
	}

	// Wait and close
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect
	newCache := make(map[string]ProbeResult)
	for res := range results {
		newCache[res.Provider] = res
	}

	// Update state
	s.cacheMu.Lock()
	s.cache = newCache
	s.lastRun = time.Now()
	s.cacheMu.Unlock()

	log.WithFields(log.Fields{
		"duration": time.Since(start),
		"found":    len(newCache),
	}).Info("provider auto-discovery completed")

	return newCache
}

// GetCachedProbes returns the latest discovery results
func (s *DiscoveryService) GetCachedProbes() map[string]ProbeResult {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.cloneCache()
}

func (s *DiscoveryService) cloneCache() map[string]ProbeResult {
	copied := make(map[string]ProbeResult, len(s.cache))
	for k, v := range s.cache {
		copied[k] = v
	}
	return copied
}
