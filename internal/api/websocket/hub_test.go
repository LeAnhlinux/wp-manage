package websocket

import (
	"testing"
	"time"
)

// TestBroadcastDoesNotBlockWhenQueueFull guards the path that could stall every
// agent's heartbeat. Broadcast is called synchronously from request handlers —
// notably the agent heartbeat handler, twice per heartbeat. If the hub's Run loop
// is wedged (a client on a stalled connection used to block WriteMessage forever)
// the 256-slot broadcast channel fills, and a blocking send would then park the
// heartbeat request itself.
//
// Run() is deliberately NOT started here: a hub with no consumer is exactly the
// wedged state being simulated.
func TestBroadcastDoesNotBlockWhenQueueFull(t *testing.T) {
	h := NewHub()

	// Overfill the queue. cap+50 sends must all return; the excess is dropped.
	const overfill = 50
	done := make(chan struct{})
	go func() {
		for i := 0; i < cap(h.broadcast)+overfill; i++ {
			h.Broadcast("dashboard:metrics", "metrics", map[string]interface{}{
				"server_id": float64(i),
			})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Broadcast blocked on a full queue — a wedged hub would stall the " +
			"agent heartbeat handler that calls this")
	}

	if got := len(h.broadcast); got != cap(h.broadcast) {
		t.Errorf("queue should be full at %d, got %d", cap(h.broadcast), got)
	}
}

// TestBroadcastCachesMetricsEvenWhenDropped checks that a dropped frame still
// updates metricsCache, so the data is not lost: the next subscriber gets the
// latest value on replay, and the next heartbeat re-broadcasts it.
func TestBroadcastCachesMetricsEvenWhenDropped(t *testing.T) {
	h := NewHub()

	for i := 0; i < cap(h.broadcast)+10; i++ {
		h.Broadcast("dashboard:metrics", "metrics", map[string]interface{}{
			"server_id":  float64(1),
			"cpu_usage":  float64(i),
			"ram_usage":  float64(0),
			"disk_usage": float64(0),
		})
	}

	h.metricsCacheMu.RLock()
	msg, ok := h.metricsCache[float64(1)]
	h.metricsCacheMu.RUnlock()
	if !ok {
		t.Fatal("metrics for server 1 were not cached")
	}
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected cached payload: %#v", msg.Data)
	}
	// The cache must hold the LAST value sent, not the last one that fit in the queue.
	if want := float64(cap(h.broadcast) + 9); data["cpu_usage"] != want {
		t.Errorf("cache holds cpu_usage=%v, want the most recent %v", data["cpu_usage"], want)
	}
}

// TestEvictServerMetrics covers the uint64/float64 key duality: server IDs arrive
// as float64 from JSON but callers pass uint64.
func TestEvictServerMetrics(t *testing.T) {
	h := NewHub()
	h.Broadcast("dashboard:metrics", "metrics", map[string]interface{}{"server_id": float64(7)})

	h.metricsCacheMu.RLock()
	_, present := h.metricsCache[float64(7)]
	h.metricsCacheMu.RUnlock()
	if !present {
		t.Fatal("precondition failed: metrics were not cached")
	}

	h.EvictServerMetrics(7)

	h.metricsCacheMu.RLock()
	_, stillThere := h.metricsCache[float64(7)]
	h.metricsCacheMu.RUnlock()
	if stillThere {
		t.Error("a deleted server's metrics are still cached and would be replayed to new subscribers")
	}
}
