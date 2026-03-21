package healthmon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPing_Reachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if !ping(context.Background(), server.URL) {
		t.Error("expected reachable=true")
	}
}

func TestPing_Unreachable(t *testing.T) {
	if ping(context.Background(), "http://localhost:19997") {
		t.Error("expected reachable=false for unreachable URL")
	}
}

func TestPing_HealthzEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if !ping(context.Background(), server.URL) {
		t.Error("expected reachable via /healthz")
	}
}

func TestPing_WellKnownFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if !ping(context.Background(), server.URL) {
		t.Error("expected reachable via /.well-known/agent.json fallback")
	}
}

func TestPing_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if ping(nil, server.URL) {
		t.Error("expected unreachable for 500 responses")
	}
}

func TestMonitor_FailureTracking(t *testing.T) {
	m := &Monitor{
		failures:   make(map[string]int),
		recoveries: make(map[string]int),
	}

	// simulate 2 failures — not enough to mark inactive
	m.failures["agent-1"] = 2
	if m.failures["agent-1"] >= maxFailures {
		t.Error("2 failures should not trigger inactive")
	}

	// 3 failures — should trigger
	m.failures["agent-1"] = 3
	if m.failures["agent-1"] < maxFailures {
		t.Error("3 failures should trigger inactive")
	}
}

func TestMonitor_RecoveryTracking(t *testing.T) {
	m := &Monitor{
		failures:   make(map[string]int),
		recoveries: make(map[string]int),
	}

	m.recoveries["agent-1"] = 1
	if m.recoveries["agent-1"] >= recoveryChecks {
		t.Error("1 recovery should not trigger active")
	}

	m.recoveries["agent-1"] = 2
	if m.recoveries["agent-1"] < recoveryChecks {
		t.Error("2 recoveries should trigger active")
	}
}

func TestConstants(t *testing.T) {
	if pingInterval <= 0 {
		t.Error("ping interval should be positive")
	}
	if pingTimeout <= 0 {
		t.Error("ping timeout should be positive")
	}
	if maxFailures <= 0 {
		t.Error("max failures should be positive")
	}
	if recoveryChecks <= 0 {
		t.Error("recovery checks should be positive")
	}
	if pingTimeout > pingInterval {
		t.Error("ping timeout should be less than ping interval")
	}
	if pingInterval != 5*time.Minute {
		t.Errorf("expected 5m interval, got %s", pingInterval)
	}
}