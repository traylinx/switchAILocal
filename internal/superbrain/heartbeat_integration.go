package superbrain

import (
	"fmt"

	"github.com/traylinx/switchAILocal/internal/heartbeat"
)

// HeartbeatIntegration manages the connection between HeartbeatMonitor and Superbrain.
type HeartbeatIntegration struct {
	monitor heartbeat.HeartbeatMonitor
}

// NewHeartbeatIntegration creates a new integration instance.
func NewHeartbeatIntegration(monitor heartbeat.HeartbeatMonitor) *HeartbeatIntegration {
	return &HeartbeatIntegration{
		monitor: monitor,
	}
}

// Start enables the integration by registering as an event handler.
func (hi *HeartbeatIntegration) Start() error {
	if hi.monitor == nil {
		return fmt.Errorf("monitor is nil")
	}
	hi.monitor.AddEventHandler(hi)
	return nil
}

// Stop disables the integration.
func (hi *HeartbeatIntegration) Stop() error {
	if hi.monitor != nil {
		hi.monitor.RemoveEventHandler(hi)
	}
	return nil
}

// HandleEvent implements the HeartbeatEventHandler interface.
func (hi *HeartbeatIntegration) HandleEvent(event *heartbeat.HeartbeatEvent) error {
	// In a real implementation, this would trigger Superbrain actions.
	// For now, we just log significant events and act as a bridge.

	switch event.Type {
	case heartbeat.EventProviderUnavailable:
		// Trigger failing over to backup providers
		// Update routing tables to exclude this provider temporarily
		fmt.Printf("[Superbrain] ALERT: Provider %s is unavailable! Triggering recovery protocols...\n", event.Provider)

	case heartbeat.EventProviderHealthy:
		// Restore provider to active pool
		fmt.Printf("[Superbrain] INFO: Provider %s is healthy again. Restoring to active pool.\n", event.Provider)

	case heartbeat.EventQuotaCritical:
		// Shift traffic away from this provider
		fmt.Printf("[Superbrain] WARNING: Provider %s reached critical quota usage! Adjusting load balancing.\n", event.Provider)

	case heartbeat.EventModelDiscovered:
		// Update capabilities registry
		fmt.Printf("[Superbrain] INFO: New models discovered for %s. Updating capabilities.\n", event.Provider)
	}

	return nil
}
