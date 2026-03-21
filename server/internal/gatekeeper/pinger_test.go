package gatekeeper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_Reachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := checkReachability(context.Background(), &http.Client{}, server.URL)

	if !result.Reachable {
		t.Errorf("expected reachable=true, got false: %s", result.Error)
	}
	if result.LatencyMs < 0 {
		t.Error("latency should be non-negative")
	}
}

func TestPing_Unreachable(t *testing.T) {
	result := checkReachability(context.Background(), &http.Client{}, "http://localhost:19998")

	if result.Reachable {
		t.Error("expected reachable=false")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestPing_A2ACompliant(t *testing.T) {
	card := map[string]interface{}{
		"name":            "Test Agent",
		"description":     "A test agent",
		"version":         "1.0.0",
		"protocolVersion": "0.3.0",
		"url":             "", // will be filled with server URL
		"skills": []map[string]interface{}{
			{"id": "test.skill", "name": "Test Skill"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			card["url"] = "http://" + r.Host
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(card)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// update URL in card to match server
	card["url"] = server.URL

	result := Ping(context.Background(), server.URL, "Test Agent", []string{"test.skill"})

	if !result.Reachable {
		t.Errorf("expected reachable=true: %s", result.Error)
	}
	if !result.A2ACompliant {
		t.Errorf("expected A2A compliant: %s", result.A2AError)
	}
	if !result.A2AMatchesCard {
		t.Errorf("expected card match: %s", result.A2AError)
	}
}

func TestPing_A2ANameMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":            "Different Agent Name",
				"protocolVersion": "0.3.0",
				"url":             "http://" + r.Host,
				"skills":          []map[string]interface{}{{"id": "test.skill", "name": "Test"}},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Ping(context.Background(), server.URL, "My Agent", []string{"test.skill"})

	if !result.A2ACompliant {
		t.Error("should be A2A compliant (valid JSON returned)")
	}
	if result.A2AMatchesCard {
		t.Error("should NOT match card — name is different")
	}
	if result.A2AError == "" {
		t.Error("should have an A2A error explaining mismatch")
	}
}

func TestPing_A2ASkillMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":            "My Agent",
				"protocolVersion": "0.3.0",
				"url":             "http://" + r.Host,
				"skills":          []map[string]interface{}{{"id": "other.skill", "name": "Other"}},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Ping(context.Background(), server.URL, "My Agent", []string{"test.skill"})

	if result.A2AMatchesCard {
		t.Error("should NOT match — submitted skill not in remote card")
	}
}

func TestPing_A2ANotExposed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Ping(context.Background(), server.URL, "My Agent", []string{"test.skill"})

	if !result.Reachable {
		t.Error("should be reachable")
	}
	if result.A2ACompliant {
		t.Error("should NOT be A2A compliant — 404 on /.well-known/agent.json")
	}
}

func TestPing_URLMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":            "My Agent",
				"protocolVersion": "0.3.0",
				"url":             "https://completely-different-domain.com", // mismatch
				"skills":          []map[string]interface{}{{"id": "test.skill", "name": "Test"}},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Ping(context.Background(), server.URL, "My Agent", []string{"test.skill"})

	if result.A2AMatchesCard {
		t.Error("should NOT match — URL in card doesn't match submitted URL")
	}
}
