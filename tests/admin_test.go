package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alexandrmotologa/pgwire-mock/internal/admin"
	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
)

func TestAdminAPIEndpoints(t *testing.T) {
	eng := mock.NewEngine(nil)
	adminSrv := admin.NewServer("127.0.0.1:0", eng, nil, false)
	if err := adminSrv.Start(); err != nil {
		t.Fatalf("failed to start admin server: %v", err)
	}
	defer adminSrv.Stop()

	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + adminSrv.Addr()

	// 1. Test Health endpoint
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// 2. Test Dynamic Rule Addition
	newRule := mock.Rule{
		ID:      "dynamic-users",
		Query:   "SELECT * FROM dynamic_users",
		Columns: []string{"id", "email"},
		Rows:    [][]string{{"1", "dyn@example.com"}},
	}
	bodyBytes, _ := json.Marshal(newRule)
	resp, err = client.Post(baseURL+"/api/rules", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to post rule: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", resp.StatusCode)
	}

	// 3. Test Match on the dynamically added rule
	matched := eng.Match("SELECT * FROM dynamic_users", nil)
	if matched.ID != "dynamic-users" {
		t.Fatalf("expected matched rule dynamic-users, got %q", matched.ID)
	}

	// 4. Test Queries endpoint
	resp, err = client.Get(baseURL + "/api/queries")
	if err != nil {
		t.Fatalf("failed to get queries: %v", err)
	}
	var queryResp struct {
		Queries []mock.QueryLog `json:"queries"`
		Count   int             `json:"count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&queryResp)
	if queryResp.Count != 1 {
		t.Errorf("expected 1 logged query, got %d", queryResp.Count)
	}

	// 5. Test Assert endpoint
	assertReq := admin.AssertRequest{
		Query: "SELECT * FROM dynamic_users",
	}
	count1 := 1
	assertReq.Count = &count1
	assertBytes, _ := json.Marshal(assertReq)
	resp, err = client.Post(baseURL+"/api/assert", "application/json", bytes.NewReader(assertBytes))
	if err != nil {
		t.Fatalf("assert failed: %v", err)
	}
	var assertResult struct {
		Passed      bool `json:"passed"`
		ActualCount int  `json:"actual_count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&assertResult)
	if !assertResult.Passed {
		t.Errorf("assertion should have passed")
	}

	// 6. Test Reset endpoint
	resp, err = client.Post(baseURL+"/api/reset", "application/json", nil)
	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if len(eng.GetLogs()) != 0 {
		t.Errorf("expected logs to be cleared")
	}

	// 7. Test Metrics endpoint
	resp, err = client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("failed to fetch metrics: %v", err)
	}
	metricsData, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(metricsData, []byte("pgwire_mock_rules_total")) {
		t.Errorf("metrics missing pgwire_mock_rules_total metric")
	}

	// 8. Test Dashboard endpoints
	resp, err = client.Get(baseURL + "/dashboard")
	if err != nil {
		t.Fatalf("failed to get dashboard: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for dashboard, got %d", resp.StatusCode)
	}
	dashHTML, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(dashHTML, []byte("PGWire-Mock Dashboard")) {
		t.Errorf("expected dashboard HTML title in response")
	}

	// Root redirects or serves dashboard
	resp, err = client.Get(baseURL + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for root dashboard, got %d", resp.StatusCode)
	}
}
