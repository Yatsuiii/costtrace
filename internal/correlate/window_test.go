package correlate

import (
	"strings"
	"testing"
	"time"

	"github.com/Yatsuiii/costtrace/internal/storage"
)

func mkDeploy(id, title string, completedAt time.Time) storage.DeployRow {
	return storage.DeployRow{
		ID:          id,
		Repo:        "Yatsuiii/costtrace",
		Branch:      "main",
		CommitSHA:   "abc1234567890",
		Title:       title,
		StartedAt:   completedAt.Add(-5 * time.Minute),
		CompletedAt: completedAt,
		Status:      "success",
	}
}

func mkAnomaly(service, date string, delta float64) storage.AnomalyRow {
	return storage.AnomalyRow{
		Service:        service,
		Date:           date,
		BaselineAmount: 100,
		ActualAmount:   100 + delta,
		Delta:          delta,
		Sigma:          3.5,
	}
}

func TestMatch_DeployWithinWindow(t *testing.T) {
	// anomaly on 2026-04-21; window is 24h ending at 2026-04-22 00:00 UTC,
	// minus 4h backward → window starts 2026-04-21 20:00 UTC.
	anomalies := []storage.AnomalyRow{mkAnomaly("EC2", "2026-04-21", 1000)}
	deployIn := mkDeploy("d1", "deploy frontend",
		time.Date(2026, 4, 21, 22, 0, 0, 0, time.UTC))
	deployOutEarly := mkDeploy("d2", "morning deploy",
		time.Date(2026, 4, 21, 8, 0, 0, 0, time.UTC))
	deployOutLate := mkDeploy("d3", "next-day deploy",
		time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC))

	got := Match(anomalies, []storage.DeployRow{deployIn, deployOutEarly, deployOutLate}, 4, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if len(got[0].Candidates) != 1 {
		t.Fatalf("expected 1 candidate (only deployIn within window), got %d", len(got[0].Candidates))
	}
	if got[0].Candidates[0].Deploy.ID != "d1" {
		t.Errorf("wrong candidate deploy: %s", got[0].Candidates[0].Deploy.ID)
	}
	if got[0].Candidates[0].Confidence != 0.05 {
		t.Errorf("time-window-only candidate should score 0.05, got %.2f", got[0].Candidates[0].Confidence)
	}
}

func TestMatch_NoDeploysInWindow(t *testing.T) {
	anomalies := []storage.AnomalyRow{mkAnomaly("EC2", "2026-04-21", 1000)}
	deployFar := mkDeploy("d1", "old deploy",
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))

	got := Match(anomalies, []storage.DeployRow{deployFar}, 4, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if len(got[0].Candidates) != 0 {
		t.Errorf("expected no candidates, got %d", len(got[0].Candidates))
	}
}

func TestMatch_LineageBoostsConfidence(t *testing.T) {
	anomalies := []storage.AnomalyRow{mkAnomaly("EC2", "2026-04-21", 1000)}
	deploy := mkDeploy("d1", "scaling deploy",
		time.Date(2026, 4, 21, 22, 0, 0, 0, time.UTC))

	ctRows := map[string][]storage.CloudTrailRow{
		"d1": {{
			EventID:      "e1",
			EventName:    "RunInstances",
			ResourceType: "AWS::EC2::Instance",
			ResourceID:   "i-0a1b2c",
			PrincipalID:  "github-actions-deploy-role",
			UserAgent:    "aws-sdk",
		}},
	}

	got := Match(anomalies, []storage.DeployRow{deploy}, 4, ctRows)
	if len(got[0].Candidates) != 1 {
		t.Fatalf("expected 1 candidate")
	}
	conf := got[0].Candidates[0].Confidence
	// 0.05 (window) + 0.6 (resource_creation) + 0.2 (principal_match) = 0.85
	if conf < 0.84 || conf > 0.86 {
		t.Errorf("expected ~0.85 confidence, got %.2f", conf)
	}
	if !strings.Contains(got[0].Candidates[0].Evidence, "resource_creation") {
		t.Errorf("evidence should mention resource_creation: %q", got[0].Candidates[0].Evidence)
	}
}

func TestMatch_SortedByConfidenceDesc(t *testing.T) {
	anomalies := []storage.AnomalyRow{mkAnomaly("EC2", "2026-04-21", 1000)}
	dHigh := mkDeploy("d-high", "actual deploy",
		time.Date(2026, 4, 21, 22, 0, 0, 0, time.UTC))
	dLow := mkDeploy("d-low", "noise deploy",
		time.Date(2026, 4, 21, 23, 0, 0, 0, time.UTC))

	ctRows := map[string][]storage.CloudTrailRow{
		"d-high": {{
			EventName:    "RunInstances",
			ResourceType: "AWS::EC2::Instance",
			ResourceID:   "i-0a1b2c",
			PrincipalID:  "github-actions",
		}},
	}

	got := Match(anomalies, []storage.DeployRow{dLow, dHigh}, 4, ctRows)
	cs := got[0].Candidates
	if len(cs) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cs))
	}
	if cs[0].Deploy.ID != "d-high" {
		t.Errorf("expected d-high first (higher confidence), got %s", cs[0].Deploy.ID)
	}
	if cs[0].Confidence <= cs[1].Confidence {
		t.Errorf("candidates not sorted desc by confidence: %.2f then %.2f", cs[0].Confidence, cs[1].Confidence)
	}
}

func TestMatch_BadAnomalyDateSkipped(t *testing.T) {
	// Malformed date should be skipped, not panic.
	anomalies := []storage.AnomalyRow{
		{Service: "EC2", Date: "not-a-date", Delta: 100},
		mkAnomaly("EC2", "2026-04-21", 500),
	}
	deploy := mkDeploy("d1", "deploy", time.Date(2026, 4, 21, 22, 0, 0, 0, time.UTC))
	got := Match(anomalies, []storage.DeployRow{deploy}, 4, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 result (bad date skipped), got %d", len(got))
	}
	if got[0].Anomaly.Date != "2026-04-21" {
		t.Errorf("wrong anomaly preserved: %s", got[0].Anomaly.Date)
	}
}
