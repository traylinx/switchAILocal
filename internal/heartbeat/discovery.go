package heartbeat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/memory"
)

// ModelDiscovery handles automatic discovery of models from providers.
type ModelDiscovery struct {
	memory   memory.MemoryManager
	lastScan map[string]time.Time
	mu       sync.RWMutex

	// eventCallback allows triggering events without direct dependency on hooks/monitor
	eventCallback func(event *HeartbeatEvent)
}

// DiscoveredModel represents a model found during discovery.
type DiscoveredModel struct {
	Provider    string    `json:"provider"`
	ModelID     string    `json:"model_id"`
	DisplayName string    `json:"display_name"`
	Size        int64     `json:"size"`
	Discovered  time.Time `json:"discovered"`
}

// NewModelDiscovery creates a new model discovery service.
func NewModelDiscovery(memory memory.MemoryManager) *ModelDiscovery {
	return &ModelDiscovery{
		memory:   memory,
		lastScan: make(map[string]time.Time),
	}
}

// SetEventCallback sets the callback for emitting discovery events.
func (d *ModelDiscovery) SetEventCallback(callback func(event *HeartbeatEvent)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.eventCallback = callback
}

// DiscoverOllamaModels discovers models from an Ollama instance.
func (d *ModelDiscovery) DiscoverOllamaModels(baseURL string) ([]*DiscoveredModel, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	resp, err := http.Get(baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name       string    `json:"name"`
			ModifiedAt time.Time `json:"modified_at"`
			Size       int64     `json:"size"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	discovered := make([]*DiscoveredModel, 0, len(result.Models))
	for _, m := range result.Models {
		discovered = append(discovered, &DiscoveredModel{
			Provider:    "ollama",
			ModelID:     m.Name,
			DisplayName: m.Name,
			Size:        m.Size,
			Discovered:  time.Now(),
		})
	}

	d.mu.Lock()
	d.lastScan["ollama"] = time.Now()

	// In a full implementation, we would compare with cached models
	// and only emit event for NEW models.
	// For now, we emit an event indicating successful discovery run.
	if d.eventCallback != nil && len(discovered) > 0 {
		d.eventCallback(&HeartbeatEvent{
			Type:      EventModelDiscovered,
			Provider:  "ollama",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"count":            len(discovered),
				"discovered_count": len(discovered),
			},
		})
	}
	d.mu.Unlock()

	return discovered, nil
}
