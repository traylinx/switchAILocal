// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traylinx/switchAILocal/internal/config"
	sdkaccess "github.com/traylinx/switchAILocal/sdk/access"
	"github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
)

// mockServiceCoordinator implements the ServiceCoordinator interface for testing.
type mockServiceCoordinator struct {
	memory    interface{}
	heartbeat interface{}
	steering  interface{}
	hooks     interface{}
	eventBus  interface{}
}

func (m *mockServiceCoordinator) GetMemory() interface{}    { return m.memory }
func (m *mockServiceCoordinator) GetHeartbeat() interface{} { return m.heartbeat }
func (m *mockServiceCoordinator) GetSteering() interface{}  { return m.steering }
func (m *mockServiceCoordinator) GetHooks() interface{}     { return m.hooks }
func (m *mockServiceCoordinator) GetEventBus() interface{}  { return m.eventBus }

// TestNewServer_WithServiceCoordinator verifies that the server can be created with a service coordinator.
func TestNewServer_WithServiceCoordinator(t *testing.T) {
	cfg := &config.Config{
		Host: "localhost",
		Port: 8080,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	mockCoordinator := &mockServiceCoordinator{
		memory:    "mock-memory",
		heartbeat: "mock-heartbeat",
		steering:  "mock-steering",
		hooks:     "mock-hooks",
		eventBus:  "mock-eventbus",
	}

	server := NewServer(
		cfg,
		authManager,
		accessManager,
		"/tmp/config.yaml",
		nil,
		WithServiceCoordinator(mockCoordinator),
	)

	assert.NotNil(t, server)
	assert.NotNil(t, server.serviceCoordinator)
	assert.Equal(t, mockCoordinator, server.serviceCoordinator)
	assert.NotNil(t, server.pipelineIntegrator, "Pipeline integrator should be created from coordinator")
	assert.NotNil(t, server.eventBusIntegrator, "Event bus integrator should be created from coordinator")
}

// TestNewServer_WithoutServiceCoordinator verifies that the server works without a service coordinator.
func TestNewServer_WithoutServiceCoordinator(t *testing.T) {
	cfg := &config.Config{
		Host: "localhost",
		Port: 8080,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	server := NewServer(
		cfg,
		authManager,
		accessManager,
		"/tmp/config.yaml",
		nil,
	)

	assert.NotNil(t, server)
	assert.Nil(t, server.serviceCoordinator)
	assert.Nil(t, server.pipelineIntegrator)
	assert.Nil(t, server.eventBusIntegrator)
}

// TestPipelineIntegratorWrapper_ApplySteering verifies the wrapper handles nil steering gracefully.
func TestPipelineIntegratorWrapper_ApplySteering(t *testing.T) {
	wrapper := &pipelineIntegratorWrapper{
		steering: nil,
		memory:   nil,
		eventBus: nil,
	}

	messages := []map[string]string{
		{"role": "user", "content": "test"},
	}

	model, modifiedMessages, err := wrapper.ApplySteering(nil, messages)

	assert.NoError(t, err)
	assert.Equal(t, "", model)
	assert.Equal(t, messages, modifiedMessages)
}

// TestPipelineIntegratorWrapper_RecordRouting verifies the wrapper handles nil memory gracefully.
func TestPipelineIntegratorWrapper_RecordRouting(t *testing.T) {
	wrapper := &pipelineIntegratorWrapper{
		steering: nil,
		memory:   nil,
		eventBus: nil,
	}

	err := wrapper.RecordRouting(nil)

	assert.NoError(t, err)
}

// TestPipelineIntegratorWrapper_UpdateOutcome verifies the wrapper handles nil memory gracefully.
func TestPipelineIntegratorWrapper_UpdateOutcome(t *testing.T) {
	wrapper := &pipelineIntegratorWrapper{
		steering: nil,
		memory:   nil,
		eventBus: nil,
	}

	err := wrapper.UpdateOutcome(nil)

	assert.NoError(t, err)
}

// TestPipelineIntegratorWrapper_EmitRoutingEvent verifies the wrapper handles nil event bus gracefully.
func TestPipelineIntegratorWrapper_EmitRoutingEvent(t *testing.T) {
	wrapper := &pipelineIntegratorWrapper{
		steering: nil,
		memory:   nil,
		eventBus: nil,
	}

	err := wrapper.EmitRoutingEvent(nil)

	assert.NoError(t, err)
}

// TestEventBusIntegratorWrapper_ConnectHeartbeatEvents verifies the wrapper handles nil components gracefully.
func TestEventBusIntegratorWrapper_ConnectHeartbeatEvents(t *testing.T) {
	wrapper := &eventBusIntegratorWrapper{
		eventBus:  nil,
		hooks:     nil,
		heartbeat: nil,
	}

	err := wrapper.ConnectHeartbeatEvents()

	assert.NoError(t, err)
}

// TestEventBusIntegratorWrapper_ConnectRoutingEvents verifies the wrapper handles nil components gracefully.
func TestEventBusIntegratorWrapper_ConnectRoutingEvents(t *testing.T) {
	wrapper := &eventBusIntegratorWrapper{
		eventBus:  nil,
		hooks:     nil,
		heartbeat: nil,
	}

	err := wrapper.ConnectRoutingEvents()

	assert.NoError(t, err)
}

// TestEventBusIntegratorWrapper_ConnectProviderEvents verifies the wrapper handles nil components gracefully.
func TestEventBusIntegratorWrapper_ConnectProviderEvents(t *testing.T) {
	wrapper := &eventBusIntegratorWrapper{
		eventBus:  nil,
		hooks:     nil,
		heartbeat: nil,
	}

	err := wrapper.ConnectProviderEvents()

	assert.NoError(t, err)
}

// TestEventBusIntegratorWrapper_EmitEvent verifies the wrapper handles nil event bus gracefully.
func TestEventBusIntegratorWrapper_EmitEvent(t *testing.T) {
	wrapper := &eventBusIntegratorWrapper{
		eventBus:  nil,
		hooks:     nil,
		heartbeat: nil,
	}

	err := wrapper.EmitEvent(nil)

	assert.NoError(t, err)
}

// TestCreatePipelineIntegratorFromCoordinator_NilCoordinator verifies nil handling.
func TestCreatePipelineIntegratorFromCoordinator_NilCoordinator(t *testing.T) {
	integrator := createPipelineIntegratorFromCoordinator(nil)

	assert.Nil(t, integrator)
}

// TestCreatePipelineIntegratorFromCoordinator_ValidCoordinator verifies integrator creation.
func TestCreatePipelineIntegratorFromCoordinator_ValidCoordinator(t *testing.T) {
	mockCoordinator := &mockServiceCoordinator{
		memory:    "mock-memory",
		heartbeat: "mock-heartbeat",
		steering:  "mock-steering",
		hooks:     "mock-hooks",
		eventBus:  "mock-eventbus",
	}

	integrator := createPipelineIntegratorFromCoordinator(mockCoordinator)

	assert.NotNil(t, integrator)
	
	// Verify the integrator can be used
	model, messages, err := integrator.ApplySteering(nil, []map[string]string{})
	assert.NoError(t, err)
	assert.Equal(t, "", model)
	assert.NotNil(t, messages)
}

// TestCreateEventBusIntegratorFromCoordinator_NilCoordinator verifies nil handling.
func TestCreateEventBusIntegratorFromCoordinator_NilCoordinator(t *testing.T) {
	integrator := createEventBusIntegratorFromCoordinator(nil)

	assert.Nil(t, integrator)
}

// TestCreateEventBusIntegratorFromCoordinator_ValidCoordinator verifies integrator creation.
func TestCreateEventBusIntegratorFromCoordinator_ValidCoordinator(t *testing.T) {
	mockCoordinator := &mockServiceCoordinator{
		memory:    "mock-memory",
		heartbeat: "mock-heartbeat",
		steering:  "mock-steering",
		hooks:     "mock-hooks",
		eventBus:  "mock-eventbus",
	}

	integrator := createEventBusIntegratorFromCoordinator(mockCoordinator)

	assert.NotNil(t, integrator)
	
	// Verify the integrator can be used
	err := integrator.ConnectHeartbeatEvents()
	assert.NoError(t, err)
}
