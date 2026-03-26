package autoroute

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ExperimentJournal maintains a TSV audit trail of all routing
// decisions and shadow experiments, directly inspired by AutoResearch.
type ExperimentJournal struct {
	filePath   string
	file       *os.File
	writer     *csv.Writer
	mu         sync.Mutex
	enabled    bool

	// In-memory ring buffer for recent UI telemetry
	recent     []JournalEntry
	recentIdx  int
	recentLen  int
	maxRecent  int
}

// JournalEntry represents a single row in the TSV log.
type JournalEntry struct {
	Timestamp      time.Time
	RequestID      string
	Intent         string
	Complexity     float64
	// Production Decision
	ProdModel      string
	ProdTier       string
	ProdLatency    time.Duration
	ProdSuccess    bool
	ProdRQS        float64
	// Shadow Decision (if diff from Prod)
	ShadowModel    string
	ShadowTier     string
	ShadowExpectedRQS float64 // Predicted RQS based on heuristic/history
	// Active Weights
	WeightAvail    float64
	WeightQuota    float64
	WeightLatency  float64
	WeightSuccess  float64
}

// NewExperimentJournal initializes an append-only TSV logger.
func NewExperimentJournal(workspaceDir string, enabled bool) (*ExperimentJournal, error) {
	if !enabled {
		return &ExperimentJournal{enabled: false}, nil
	}

	logDir := filepath.Join(workspaceDir, "logs", "autoroute")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create journal directory: %w", err)
	}

	filePath := filepath.Join(logDir, "experiments.tsv")
	
	// Open file in append mode, create if not exist
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open journal file: %w", err)
	}

	// Check if we need to write the header
	stat, err := file.Stat()
	needsHeader := err == nil && stat.Size() == 0

	writer := csv.NewWriter(file)
	writer.Comma = '\t'

	journal := &ExperimentJournal{
		filePath:  filePath,
		file:      file,
		writer:    writer,
		enabled:   true,
		maxRecent: 100, // Keep last 100 entries for the Management UI
		recent:    make([]JournalEntry, 100),
	}

	if needsHeader {
		headers := []string{
			"timestamp", "request_id", "intent", "complexity",
			"prod_model", "prod_tier", "prod_latency_ms", "prod_success", "prod_rqs",
			"shadow_model", "shadow_tier", "shadow_exp_rqs",
			"w_avail", "w_quota", "w_lat", "w_succ",
		}
		journal.mu.Lock()
		writer.Write(headers)
		writer.Flush()
		journal.mu.Unlock()
	}

	return journal, nil
}

// Record appends a new entry to the TSV journal.
func (j *ExperimentJournal) Record(entry JournalEntry) {
	if !j.enabled || j.writer == nil {
		return
	}

	row := []string{
		entry.Timestamp.Format(time.RFC3339),
		entry.RequestID,
		entry.Intent,
		fmt.Sprintf("%.3f", entry.Complexity),
		entry.ProdModel,
		entry.ProdTier,
		fmt.Sprintf("%d", entry.ProdLatency.Milliseconds()),
		fmt.Sprintf("%t", entry.ProdSuccess),
		fmt.Sprintf("%.3f", entry.ProdRQS),
		entry.ShadowModel,
		entry.ShadowTier,
		fmt.Sprintf("%.3f", entry.ShadowExpectedRQS),
		fmt.Sprintf("%.3f", entry.WeightAvail),
		fmt.Sprintf("%.3f", entry.WeightQuota),
		fmt.Sprintf("%.3f", entry.WeightLatency),
		fmt.Sprintf("%.3f", entry.WeightSuccess),
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	
	// Update in-memory ring buffer
	j.recent[j.recentIdx] = entry
	j.recentIdx = (j.recentIdx + 1) % j.maxRecent
	if j.recentLen < j.maxRecent {
		j.recentLen++
	}
	
	if err := j.writer.Write(row); err != nil {
		log.WithError(err).Warn("Failed to write to experiment journal")
		return
	}
	j.writer.Flush()
}

// Close gracefully flushes and closes the journal file.
func (j *ExperimentJournal) Close() error {
	if !j.enabled || j.file == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.writer.Flush()
	return j.file.Close()
}

// GetRecent returns up to the last n entries from the in-memory ring buffer,
// ordered from newest to oldest.
func (j *ExperimentJournal) GetRecent(n int) []JournalEntry {
	if !j.enabled {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	if n > j.recentLen {
		n = j.recentLen
	}
	if n == 0 {
		return []JournalEntry{}
	}

	result := make([]JournalEntry, n)
	
	// Read backwards from the current index
	idx := j.recentIdx - 1
	for i := 0; i < n; i++ {
		if idx < 0 {
			idx = j.maxRecent - 1
		}
		result[i] = j.recent[idx]
		idx--
	}
	
	return result
}
