package source

import (
	"sync"
	"testing"
	"time"
)

func TestTakeoverRaceOrdering(t *testing.T) {
	registry := newConnectionRegistry()
	token := "race-token"

	// 1. client1 (seq 1) starts
	conn1 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 1,
	}

	// 2. client2 (seq 2) starts
	conn2 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 2,
	}

	// Simulating the race where client2 commits first
	evictedByConn2 := registry.commitConnection(token, conn2)
	if evictedByConn2 != nil {
		t.Error("client2 should not evict anything since registry was empty")
	}

	// Assert client2 is in the registry
	if registry.getActiveConnection(token) != conn2 {
		t.Error("client2 should be registered as active")
	}

	// Now client1 (seq 1, older) commits second
	evictedByConn1 := registry.commitConnection(token, conn1)
	if evictedByConn1 != nil {
		t.Error("client1 should not evict client2 since client1 is older")
	}

	// Assert client2 remains registered
	if registry.getActiveConnection(token) != conn2 {
		t.Error("client2 should remain registered as active connection")
	}

	// Assert client1 got closed (evicted itself)
	select {
	case <-conn1.closed:
		// Passed, client1 evicted itself
	case <-time.After(100 * time.Millisecond):
		t.Error("client1 should have evicted/closed itself")
	}

	// Assert client2 is not closed
	select {
	case <-conn2.closed:
		t.Error("client2 should not be closed")
	default:
		// Passed
	}
}

func TestNormalTakeoverOrdering(t *testing.T) {
	registry := newConnectionRegistry()
	token := "normal-token"

	// 1. client1 (seq 1) commits first
	conn1 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 1,
	}
	evictedByConn1 := registry.commitConnection(token, conn1)
	if evictedByConn1 != nil {
		t.Error("client1 should not evict anything since registry was empty")
	}

	// 2. client2 (seq 2) commits second
	conn2 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 2,
	}
	evictedByConn2 := registry.commitConnection(token, conn2)
	if evictedByConn2 != conn1 {
		t.Error("client2 should evict client1")
	}

	// Assert client2 is registered as active
	if registry.getActiveConnection(token) != conn2 {
		t.Error("client2 should be registered as active")
	}
}

func TestTakeoverBindingDoubleCheckRace(t *testing.T) {
	netRegistry := newConnectionRegistry()
	token := "race-double-check-token"
	
	// 1. Initialize a dummy workspace
	workspace := &Workspace{
		id:              token,
		ptys:            newOrderedPTYMap(),
		pendingCount:    make(map[uint16]int),
		schedulerSignal: make(chan struct{}, 1),
	}
	workspace.backpressureCond = sync.NewCond(&workspace.mutex)

	// 2. conn1 (sequence 1) connects and binds initially
	conn1 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 1,
	}
	netRegistry.commitConnection(token, conn1)
	workspace.PerformReplayTakeover(conn1)

	if workspace.GetSocketWriter() != conn1 {
		t.Fatalf("Expected conn1 to be bound initially")
	}

	// 3. conn2 (sequence 2) connects and commits in netRegistry, evicting conn1
	conn2 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 2,
	}
	if evicted := netRegistry.commitConnection(token, conn2); evicted != conn1 {
		t.Fatalf("Expected conn1 to be evicted by conn2")
	}

	// 4. Before conn2 can bind to the workspace, conn3 (sequence 3) arrives,
	// commits in netRegistry, and evicts conn2
	conn3 := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: 3,
	}
	if evicted := netRegistry.commitConnection(token, conn3); evicted != conn2 {
		t.Fatalf("Expected conn2 to be evicted by conn3")
	}

	// 5. Simulating conn2 executing its double-checked takeover check
	netRegistry.mutex.Lock()
	if netRegistry.connections[token] == conn2 {
		workspace.PerformReplayTakeover(conn2)
	}
	netRegistry.mutex.Unlock()

	// Assert conn2 was skipped and did NOT overwrite the workspace writer
	if workspace.GetSocketWriter() == conn2 {
		t.Errorf("Security breach: conn2 should not have bound because it was evicted by conn3")
	}

	// 6. Simulating conn3 executing its double-checked takeover check
	netRegistry.mutex.Lock()
	if netRegistry.connections[token] == conn3 {
		workspace.PerformReplayTakeover(conn3)
	}
	netRegistry.mutex.Unlock()

	// Assert conn3 successfully bound
	if workspace.GetSocketWriter() != conn3 {
		t.Errorf("Expected conn3 to be bound as the active writer")
	}

	// 7. Simulating conn2 exiting and running its cleanup defer block
	workspace.ClearSocketWriter(conn2)

	// Assert conn3 is STILL bound (conn2's cleanup must be a no-op on conn3)
	if workspace.GetSocketWriter() != conn3 {
		t.Errorf("Expected conn3 to remain bound after conn2's cleanup")
	}
}

