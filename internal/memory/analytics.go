package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AnalyticsEngine computes and stores aggregated metrics from routing history.
// It provides provider stats, model performance metrics, and trend analysis.
type AnalyticsEngine struct {
	analyticsDir string
	
	// Cached data
	providerStats    map[string]*ProviderStats
	modelPerformance map[string]*ModelPerformance
	
	// Synchronization
	mu sync.RWMutex
	
	// Last update tracking
	lastUpdate time.Time
}

// AnalyticsSummary provides a comprehensive view of system analytics.
type AnalyticsSummary struct {
	GeneratedAt      time.Time                    `json:"generated_at"`
	TimeRange        TimeRange                    `json:"time_range"`
	ProviderStats    map[string]*ProviderStats    `json:"provider_stats"`
	ModelPerformance map[string]*ModelPerformance `json:"model_performance"`
	TierEffectiveness *TierEffectiveness          `json:"tier_effectiveness"`
	CostAnalysis     *CostAnalysis               `json:"cost_analysis"`
	TrendAnalysis    *TrendAnalysis              `json:"trend_analysis"`
}

// TimeRange represents a time period for analytics.
type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Days      int       `json:"days"`
}

// TierEffectiveness tracks routing tier performance.
type TierEffectiveness struct {
	ReflexTier    *TierStats `json:"reflex_tier"`
	SemanticTier  *TierStats `json:"semantic_tier"`
	CognitiveTier *TierStats `json:"cognitive_tier"`
	LearnedTier   *TierStats `json:"learned_tier"`
}

// TierStats provides statistics for a routing tier.
type TierStats struct {
	TotalRequests int     `json:"total_requests"`
	SuccessRate   float64 `json:"success_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	AvgConfidence float64 `json:"avg_confidence"`
}

// CostAnalysis provides cost-related analytics.
type CostAnalysis struct {
	TotalCost        float64            `json:"total_cost"`
	CostByProvider   map[string]float64 `json:"cost_by_provider"`
	CostByModel      map[string]float64 `json:"cost_by_model"`
	AvgCostPerReq    float64            `json:"avg_cost_per_req"`
	CostTrend        []DailyCost        `json:"cost_trend"`
	SavingsFromLocal float64            `json:"savings_from_local"`
}

// DailyCost represents cost for a specific day.
type DailyCost struct {
	Date string  `json:"date"`
	Cost float64 `json:"cost"`
}

// TrendAnalysis provides trend information over time.
type TrendAnalysis struct {
	RequestVolumeTrend []DailyVolume    `json:"request_volume_trend"`
	SuccessRateTrend   []DailyMetric    `json:"success_rate_trend"`
	LatencyTrend       []DailyMetric    `json:"latency_trend"`
	PopularModels      []ModelPopularity `json:"popular_models"`
	PeakHours          []HourlyStats    `json:"peak_hours"`
}

// DailyVolume represents request volume for a specific day.
type DailyVolume struct {
	Date     string `json:"date"`
	Requests int    `json:"requests"`
}

// DailyMetric represents a daily metric value.
type DailyMetric struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// ModelPopularity represents model usage statistics.
type ModelPopularity struct {
	Model       string  `json:"model"`
	Requests    int     `json:"requests"`
	Percentage  float64 `json:"percentage"`
	SuccessRate float64 `json:"success_rate"`
}

// HourlyStats represents statistics for a specific hour of day.
type HourlyStats struct {
	Hour     int     `json:"hour"`
	Requests int     `json:"requests"`
	AvgLatency float64 `json:"avg_latency"`
}

// NewAnalyticsEngine creates a new analytics engine.
func NewAnalyticsEngine(analyticsDir string) *AnalyticsEngine {
	return &AnalyticsEngine{
		analyticsDir:     analyticsDir,
		providerStats:    make(map[string]*ProviderStats),
		modelPerformance: make(map[string]*ModelPerformance),
	}
}

// ComputeAnalytics computes analytics from routing decisions and stores the results.
func (ae *AnalyticsEngine) ComputeAnalytics(decisions []*RoutingDecision) (*AnalyticsSummary, error) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	if len(decisions) == 0 {
		return ae.createEmptyAnalytics(), nil
	}
	
	// Determine time range
	timeRange := ae.calculateTimeRange(decisions)
	
	// Compute provider statistics
	providerStats := ae.computeProviderStats(decisions)
	
	// Compute model performance
	modelPerformance := ae.computeModelPerformance(decisions)
	
	// Compute tier effectiveness
	tierEffectiveness := ae.computeTierEffectiveness(decisions)
	
	// Compute cost analysis
	costAnalysis := ae.computeCostAnalysis(decisions)
	
	// Compute trend analysis
	trendAnalysis := ae.computeTrendAnalysis(decisions)
	
	// Create summary
	summary := &AnalyticsSummary{
		GeneratedAt:       time.Now(),
		TimeRange:         timeRange,
		ProviderStats:     providerStats,
		ModelPerformance:  modelPerformance,
		TierEffectiveness: tierEffectiveness,
		CostAnalysis:      costAnalysis,
		TrendAnalysis:     trendAnalysis,
	}
	
	// Cache results
	ae.providerStats = providerStats
	ae.modelPerformance = modelPerformance
	ae.lastUpdate = time.Now()
	
	// Store results to disk
	if err := ae.storeAnalytics(summary); err != nil {
		return nil, fmt.Errorf("failed to store analytics: %w", err)
	}
	
	return summary, nil
}

// calculateTimeRange determines the time range covered by the decisions.
func (ae *AnalyticsEngine) calculateTimeRange(decisions []*RoutingDecision) TimeRange {
	if len(decisions) == 0 {
		now := time.Now()
		return TimeRange{
			StartTime: now,
			EndTime:   now,
			Days:      0,
		}
	}
	
	// Find earliest and latest timestamps
	earliest := decisions[0].Timestamp
	latest := decisions[0].Timestamp
	
	for _, decision := range decisions {
		if decision.Timestamp.Before(earliest) {
			earliest = decision.Timestamp
		}
		if decision.Timestamp.After(latest) {
			latest = decision.Timestamp
		}
	}
	
	days := int(latest.Sub(earliest).Hours()/24) + 1
	
	return TimeRange{
		StartTime: earliest,
		EndTime:   latest,
		Days:      days,
	}
}

// computeProviderStats computes statistics for each provider.
func (ae *AnalyticsEngine) computeProviderStats(decisions []*RoutingDecision) map[string]*ProviderStats {
	providerData := make(map[string]*providerStatsAccumulator)
	
	// Accumulate data for each provider
	for _, decision := range decisions {
		provider := ae.extractProvider(decision.Routing.SelectedModel)
		
		if _, exists := providerData[provider]; !exists {
			providerData[provider] = &providerStatsAccumulator{}
		}
		
		acc := providerData[provider]
		acc.totalRequests++
		acc.totalLatency += float64(decision.Outcome.ResponseTimeMs)
		
		if decision.Outcome.Success {
			acc.successfulRequests++
		} else {
			acc.failedRequests++
		}
	}
	
	// Convert to final stats
	stats := make(map[string]*ProviderStats)
	for provider, acc := range providerData {
		successRate := 0.0
		if acc.totalRequests > 0 {
			successRate = float64(acc.successfulRequests) / float64(acc.totalRequests)
		}
		
		avgLatency := 0.0
		if acc.totalRequests > 0 {
			avgLatency = acc.totalLatency / float64(acc.totalRequests)
		}
		
		errorRate := 0.0
		if acc.totalRequests > 0 {
			errorRate = float64(acc.failedRequests) / float64(acc.totalRequests)
		}
		
		stats[provider] = &ProviderStats{
			Provider:      provider,
			TotalRequests: acc.totalRequests,
			SuccessRate:   successRate,
			AvgLatencyMs:  avgLatency,
			ErrorRate:     errorRate,
			LastUpdated:   time.Now(),
		}
	}
	
	return stats
}

// computeModelPerformance computes performance metrics for each model.
func (ae *AnalyticsEngine) computeModelPerformance(decisions []*RoutingDecision) map[string]*ModelPerformance {
	modelData := make(map[string]*modelPerformanceAccumulator)
	
	// Accumulate data for each model
	for _, decision := range decisions {
		model := decision.Routing.SelectedModel
		
		if _, exists := modelData[model]; !exists {
			modelData[model] = &modelPerformanceAccumulator{}
		}
		
		acc := modelData[model]
		acc.totalRequests++
		acc.totalQuality += decision.Outcome.QualityScore
		
		if decision.Outcome.Success {
			acc.successfulRequests++
		}
		
		// Estimate cost (simplified - in real implementation, this would use actual pricing)
		acc.totalCost += ae.estimateRequestCost(model, decision)
	}
	
	// Convert to final performance metrics
	performance := make(map[string]*ModelPerformance)
	for model, acc := range modelData {
		successRate := 0.0
		if acc.totalRequests > 0 {
			successRate = float64(acc.successfulRequests) / float64(acc.totalRequests)
		}
		
		avgQuality := 0.0
		if acc.totalRequests > 0 {
			avgQuality = acc.totalQuality / float64(acc.totalRequests)
		}
		
		avgCost := 0.0
		if acc.totalRequests > 0 {
			avgCost = acc.totalCost / float64(acc.totalRequests)
		}
		
		performance[model] = &ModelPerformance{
			Model:           model,
			TotalRequests:   acc.totalRequests,
			SuccessRate:     successRate,
			AvgQualityScore: avgQuality,
			AvgCostPerReq:   avgCost,
		}
	}
	
	return performance
}

// computeTierEffectiveness computes effectiveness metrics for each routing tier.
func (ae *AnalyticsEngine) computeTierEffectiveness(decisions []*RoutingDecision) *TierEffectiveness {
	tierData := make(map[string]*tierStatsAccumulator)
	
	// Initialize accumulators for all tiers
	tiers := []string{"reflex", "semantic", "cognitive", "learned"}
	for _, tier := range tiers {
		tierData[tier] = &tierStatsAccumulator{}
	}
	
	// Accumulate data for each tier
	for _, decision := range decisions {
		tier := decision.Routing.Tier
		
		if acc, exists := tierData[tier]; exists {
			acc.totalRequests++
			acc.totalLatency += float64(decision.Routing.LatencyMs)
			acc.totalConfidence += decision.Routing.Confidence
			
			if decision.Outcome.Success {
				acc.successfulRequests++
			}
		}
	}
	
	// Convert to final tier stats
	return &TierEffectiveness{
		ReflexTier:    ae.convertTierStats(tierData["reflex"]),
		SemanticTier:  ae.convertTierStats(tierData["semantic"]),
		CognitiveTier: ae.convertTierStats(tierData["cognitive"]),
		LearnedTier:   ae.convertTierStats(tierData["learned"]),
	}
}

// computeCostAnalysis computes cost-related analytics.
func (ae *AnalyticsEngine) computeCostAnalysis(decisions []*RoutingDecision) *CostAnalysis {
	var totalCost float64
	costByProvider := make(map[string]float64)
	costByModel := make(map[string]float64)
	dailyCosts := make(map[string]float64)
	var localRequests int
	
	for _, decision := range decisions {
		cost := ae.estimateRequestCost(decision.Routing.SelectedModel, decision)
		totalCost += cost
		
		// Cost by provider
		provider := ae.extractProvider(decision.Routing.SelectedModel)
		costByProvider[provider] += cost
		
		// Cost by model
		costByModel[decision.Routing.SelectedModel] += cost
		
		// Daily costs
		date := decision.Timestamp.Format("2006-01-02")
		dailyCosts[date] += cost
		
		// Count local requests for savings calculation
		if ae.isLocalProvider(provider) {
			localRequests++
		}
	}
	
	// Calculate average cost per request
	avgCostPerReq := 0.0
	if len(decisions) > 0 {
		avgCostPerReq = totalCost / float64(len(decisions))
	}
	
	// Convert daily costs to sorted slice
	var costTrend []DailyCost
	for date, cost := range dailyCosts {
		costTrend = append(costTrend, DailyCost{
			Date: date,
			Cost: cost,
		})
	}
	sort.Slice(costTrend, func(i, j int) bool {
		return costTrend[i].Date < costTrend[j].Date
	})
	
	// Estimate savings from local models (assume cloud cost would be $0.01 per request)
	savingsFromLocal := float64(localRequests) * 0.01
	
	return &CostAnalysis{
		TotalCost:        totalCost,
		CostByProvider:   costByProvider,
		CostByModel:      costByModel,
		AvgCostPerReq:    avgCostPerReq,
		CostTrend:        costTrend,
		SavingsFromLocal: savingsFromLocal,
	}
}

// computeTrendAnalysis computes trend information over time.
func (ae *AnalyticsEngine) computeTrendAnalysis(decisions []*RoutingDecision) *TrendAnalysis {
	// Request volume trend
	dailyVolume := make(map[string]int)
	dailySuccess := make(map[string]*dailySuccessAccumulator)
	dailyLatency := make(map[string]*dailyLatencyAccumulator)
	modelCounts := make(map[string]int)
	hourlyCounts := make(map[int]*hourlyStatsAccumulator)
	
	// Initialize hourly accumulators
	for i := 0; i < 24; i++ {
		hourlyCounts[i] = &hourlyStatsAccumulator{}
	}
	
	for _, decision := range decisions {
		date := decision.Timestamp.Format("2006-01-02")
		hour := decision.Timestamp.Hour()
		
		// Daily volume
		dailyVolume[date]++
		
		// Daily success rate
		if _, exists := dailySuccess[date]; !exists {
			dailySuccess[date] = &dailySuccessAccumulator{}
		}
		dailySuccess[date].total++
		if decision.Outcome.Success {
			dailySuccess[date].successful++
		}
		
		// Daily latency
		if _, exists := dailyLatency[date]; !exists {
			dailyLatency[date] = &dailyLatencyAccumulator{}
		}
		dailyLatency[date].total++
		dailyLatency[date].totalLatency += float64(decision.Outcome.ResponseTimeMs)
		
		// Model popularity
		modelCounts[decision.Routing.SelectedModel]++
		
		// Hourly stats
		hourlyCounts[hour].requests++
		hourlyCounts[hour].totalLatency += float64(decision.Outcome.ResponseTimeMs)
	}
	
	// Convert to trend data
	var volumeTrend []DailyVolume
	for date, count := range dailyVolume {
		volumeTrend = append(volumeTrend, DailyVolume{
			Date:     date,
			Requests: count,
		})
	}
	sort.Slice(volumeTrend, func(i, j int) bool {
		return volumeTrend[i].Date < volumeTrend[j].Date
	})
	
	var successTrend []DailyMetric
	for date, acc := range dailySuccess {
		rate := 0.0
		if acc.total > 0 {
			rate = float64(acc.successful) / float64(acc.total)
		}
		successTrend = append(successTrend, DailyMetric{
			Date:  date,
			Value: rate,
		})
	}
	sort.Slice(successTrend, func(i, j int) bool {
		return successTrend[i].Date < successTrend[j].Date
	})
	
	var latencyTrend []DailyMetric
	for date, acc := range dailyLatency {
		avgLatency := 0.0
		if acc.total > 0 {
			avgLatency = acc.totalLatency / float64(acc.total)
		}
		latencyTrend = append(latencyTrend, DailyMetric{
			Date:  date,
			Value: avgLatency,
		})
	}
	sort.Slice(latencyTrend, func(i, j int) bool {
		return latencyTrend[i].Date < latencyTrend[j].Date
	})
	
	// Popular models
	var popularModels []ModelPopularity
	totalRequests := len(decisions)
	for model, count := range modelCounts {
		percentage := 0.0
		if totalRequests > 0 {
			percentage = float64(count) / float64(totalRequests) * 100
		}
		
		// Calculate success rate for this model
		successRate := 0.0
		if perf, exists := ae.modelPerformance[model]; exists {
			successRate = perf.SuccessRate
		}
		
		popularModels = append(popularModels, ModelPopularity{
			Model:       model,
			Requests:    count,
			Percentage:  percentage,
			SuccessRate: successRate,
		})
	}
	sort.Slice(popularModels, func(i, j int) bool {
		return popularModels[i].Requests > popularModels[j].Requests
	})
	
	// Peak hours
	var peakHours []HourlyStats
	for hour, acc := range hourlyCounts {
		avgLatency := 0.0
		if acc.requests > 0 {
			avgLatency = acc.totalLatency / float64(acc.requests)
		}
		
		peakHours = append(peakHours, HourlyStats{
			Hour:       hour,
			Requests:   acc.requests,
			AvgLatency: avgLatency,
		})
	}
	sort.Slice(peakHours, func(i, j int) bool {
		return peakHours[i].Requests > peakHours[j].Requests
	})
	
	return &TrendAnalysis{
		RequestVolumeTrend: volumeTrend,
		SuccessRateTrend:   successTrend,
		LatencyTrend:       latencyTrend,
		PopularModels:      popularModels,
		PeakHours:          peakHours,
	}
}

// Helper functions and accumulators

type providerStatsAccumulator struct {
	totalRequests      int
	successfulRequests int
	failedRequests     int
	totalLatency       float64
}

type modelPerformanceAccumulator struct {
	totalRequests      int
	successfulRequests int
	totalQuality       float64
	totalCost          float64
}

type tierStatsAccumulator struct {
	totalRequests      int
	successfulRequests int
	totalLatency       float64
	totalConfidence    float64
}

type dailySuccessAccumulator struct {
	total      int
	successful int
}

type dailyLatencyAccumulator struct {
	total        int
	totalLatency float64
}

type hourlyStatsAccumulator struct {
	requests     int
	totalLatency float64
}

// convertTierStats converts tier accumulator to final stats.
func (ae *AnalyticsEngine) convertTierStats(acc *tierStatsAccumulator) *TierStats {
	successRate := 0.0
	if acc.totalRequests > 0 {
		successRate = float64(acc.successfulRequests) / float64(acc.totalRequests)
	}
	
	avgLatency := 0.0
	if acc.totalRequests > 0 {
		avgLatency = acc.totalLatency / float64(acc.totalRequests)
	}
	
	avgConfidence := 0.0
	if acc.totalRequests > 0 {
		avgConfidence = acc.totalConfidence / float64(acc.totalRequests)
	}
	
	return &TierStats{
		TotalRequests: acc.totalRequests,
		SuccessRate:   successRate,
		AvgLatencyMs:  avgLatency,
		AvgConfidence: avgConfidence,
	}
}

// extractProvider extracts provider name from model string.
func (ae *AnalyticsEngine) extractProvider(model string) string {
	// Extract provider from model string (e.g., "claudecli:claude-sonnet-4" -> "claudecli")
	if idx := strings.Index(model, ":"); idx != -1 {
		return model[:idx]
	}
	return model
}

// isLocalProvider checks if a provider is local (no cost).
func (ae *AnalyticsEngine) isLocalProvider(provider string) bool {
	localProviders := []string{"ollama", "lmstudio", "localai"}
	for _, local := range localProviders {
		if provider == local {
			return true
		}
	}
	return false
}

// estimateRequestCost estimates the cost of a request (simplified).
func (ae *AnalyticsEngine) estimateRequestCost(model string, decision *RoutingDecision) float64 {
	provider := ae.extractProvider(model)
	
	// Local providers have no cost
	if ae.isLocalProvider(provider) {
		return 0.0
	}
	
	// Simplified cost estimation based on provider and content length
	baseCost := 0.001 // $0.001 base cost
	
	switch provider {
	case "claudecli", "claude":
		return baseCost * 2.0 // Claude is more expensive
	case "geminicli", "gemini":
		return baseCost * 1.5 // Gemini is moderately expensive
	case "openai":
		return baseCost * 1.8 // OpenAI is expensive
	default:
		return baseCost
	}
}

// createEmptyAnalytics creates an empty analytics summary.
func (ae *AnalyticsEngine) createEmptyAnalytics() *AnalyticsSummary {
	now := time.Now()
	return &AnalyticsSummary{
		GeneratedAt: now,
		TimeRange: TimeRange{
			StartTime: now,
			EndTime:   now,
			Days:      0,
		},
		ProviderStats:     make(map[string]*ProviderStats),
		ModelPerformance:  make(map[string]*ModelPerformance),
		TierEffectiveness: &TierEffectiveness{},
		CostAnalysis:      &CostAnalysis{},
		TrendAnalysis:     &TrendAnalysis{},
	}
}

// storeAnalytics stores analytics results to disk.
func (ae *AnalyticsEngine) storeAnalytics(summary *AnalyticsSummary) error {
	// Store provider stats
	providerStatsPath := filepath.Join(ae.analyticsDir, "provider-stats.json")
	if err := ae.writeJSONFile(providerStatsPath, summary.ProviderStats); err != nil {
		return fmt.Errorf("failed to store provider stats: %w", err)
	}
	
	// Store model performance
	modelPerfPath := filepath.Join(ae.analyticsDir, "model-performance.json")
	if err := ae.writeJSONFile(modelPerfPath, summary.ModelPerformance); err != nil {
		return fmt.Errorf("failed to store model performance: %w", err)
	}
	
	// Store complete summary
	summaryPath := filepath.Join(ae.analyticsDir, "analytics-summary.json")
	if err := ae.writeJSONFile(summaryPath, summary); err != nil {
		return fmt.Errorf("failed to store analytics summary: %w", err)
	}
	
	return nil
}

// writeJSONFile writes data to a JSON file.
func (ae *AnalyticsEngine) writeJSONFile(path string, data interface{}) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	
	return nil
}

// LoadAnalytics loads previously stored analytics from disk.
func (ae *AnalyticsEngine) LoadAnalytics() (*AnalyticsSummary, error) {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	summaryPath := filepath.Join(ae.analyticsDir, "analytics-summary.json")
	
	file, err := os.Open(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ae.createEmptyAnalytics(), nil
		}
		return nil, fmt.Errorf("failed to open analytics summary: %w", err)
	}
	defer file.Close()
	
	var summary AnalyticsSummary
	decoder := json.NewDecoder(file)
	
	if err := decoder.Decode(&summary); err != nil {
		return nil, fmt.Errorf("failed to decode analytics summary: %w", err)
	}
	
	return &summary, nil
}

// GetProviderStats returns cached provider statistics.
func (ae *AnalyticsEngine) GetProviderStats() map[string]*ProviderStats {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	// Return copy to prevent external modification
	stats := make(map[string]*ProviderStats)
	for k, v := range ae.providerStats {
		stats[k] = v
	}
	
	return stats
}

// GetModelPerformance returns cached model performance metrics.
func (ae *AnalyticsEngine) GetModelPerformance() map[string]*ModelPerformance {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	// Return copy to prevent external modification
	performance := make(map[string]*ModelPerformance)
	for k, v := range ae.modelPerformance {
		performance[k] = v
	}
	
	return performance
}

// GetLastUpdate returns the timestamp of the last analytics update.
func (ae *AnalyticsEngine) GetLastUpdate() time.Time {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	return ae.lastUpdate
}