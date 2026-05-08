package detect

import (
	"testing"

	"github.com/Yatsuiii/costtrace/internal/storage"
)

func dailyRow(date, service string, amount float64) storage.CostRow {
	return storage.CostRow{Date: date, Service: service, UsageType: "BoxUsage", Amount: amount, Currency: "USD"}
}

func TestDetect_StableSeriesNoAnomaly(t *testing.T) {
	rows := []storage.CostRow{
		dailyRow("2026-01-01", "EC2", 100),
		dailyRow("2026-01-02", "EC2", 100),
		dailyRow("2026-01-03", "EC2", 100),
		dailyRow("2026-01-04", "EC2", 100),
		dailyRow("2026-01-05", "EC2", 100),
		dailyRow("2026-01-06", "EC2", 100),
		dailyRow("2026-01-07", "EC2", 100),
		dailyRow("2026-01-08", "EC2", 100), // candidate day, identical to baseline
	}
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 1})
	if len(got) != 0 {
		t.Fatalf("expected no anomalies on flat series, got %d: %+v", len(got), got)
	}
}

func TestDetect_FlagsClearSpike(t *testing.T) {
	rows := []storage.CostRow{
		dailyRow("2026-01-01", "EC2", 100),
		dailyRow("2026-01-02", "EC2", 105),
		dailyRow("2026-01-03", "EC2", 95),
		dailyRow("2026-01-04", "EC2", 100),
		dailyRow("2026-01-05", "EC2", 102),
		dailyRow("2026-01-06", "EC2", 98),
		dailyRow("2026-01-07", "EC2", 100),
		dailyRow("2026-01-08", "EC2", 1000), // 10x spike
	}
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(got))
	}
	a := got[0]
	if a.Service != "EC2" || a.Date != "2026-01-08" {
		t.Errorf("wrong anomaly target: %+v", a)
	}
	if a.Sigma < 2.0 {
		t.Errorf("sigma should be >= threshold, got %.2f", a.Sigma)
	}
	if a.Delta < 800 {
		t.Errorf("delta should be ~900, got %.2f", a.Delta)
	}
}

func TestDetect_MinDeltaUSDFloor(t *testing.T) {
	// 7 baseline days at $1, then $5 — sigma is huge but delta is only $4.
	rows := []storage.CostRow{
		dailyRow("2026-01-01", "S3", 1),
		dailyRow("2026-01-02", "S3", 1),
		dailyRow("2026-01-03", "S3", 1),
		dailyRow("2026-01-04", "S3", 1),
		dailyRow("2026-01-05", "S3", 1),
		dailyRow("2026-01-06", "S3", 1),
		dailyRow("2026-01-07", "S3", 1),
		dailyRow("2026-01-08", "S3", 5),
	}
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) != 0 {
		t.Fatalf("expected MinDeltaUSD floor to suppress, got %d anomalies: %+v", len(got), got)
	}
}

func TestDetect_AggregatesUsageTypesPerDay(t *testing.T) {
	// EC2 has two usage types per day; Detect must sum them before computing.
	mk := func(date, ut string, amt float64) storage.CostRow {
		return storage.CostRow{Date: date, Service: "EC2", UsageType: ut, Amount: amt, Currency: "USD"}
	}
	// small per-day jitter so the baseline window has stddev > 0
	jitter := []float64{0, 2, -1, 1, -2, 1, 0}
	var rows []storage.CostRow
	for i := 1; i <= 7; i++ {
		date := "2026-01-0" + string(rune('0'+i))
		half := 50 + jitter[i-1]/2
		rows = append(rows, mk(date, "BoxUsage", half), mk(date, "EBS", half))
	}
	rows = append(rows,
		mk("2026-01-08", "BoxUsage", 500),
		mk("2026-01-08", "EBS", 500), // total $1000
	)
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) != 1 {
		t.Fatalf("expected 1 anomaly with usage_type aggregation, got %d", len(got))
	}
	if got[0].ActualAmount != 1000 {
		t.Errorf("expected actual=1000, got %.2f", got[0].ActualAmount)
	}
}

func TestDetect_RespectsBaselineWindow(t *testing.T) {
	// 8 baseline days with small variance, then a spike on day 9.
	// With BaselineDays=7, day 9's window is days 2-8.
	rows := []storage.CostRow{
		dailyRow("2026-01-01", "EC2", 100),
		dailyRow("2026-01-02", "EC2", 102),
		dailyRow("2026-01-03", "EC2", 98),
		dailyRow("2026-01-04", "EC2", 101),
		dailyRow("2026-01-05", "EC2", 99),
		dailyRow("2026-01-06", "EC2", 100),
		dailyRow("2026-01-07", "EC2", 103),
		dailyRow("2026-01-08", "EC2", 97),
		dailyRow("2026-01-09", "EC2", 1000),
	}
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(got))
	}
	if got[0].Date != "2026-01-09" {
		t.Errorf("wrong anomaly date: %s", got[0].Date)
	}
}

func TestDetect_MultipleServices(t *testing.T) {
	// Both services have small baseline jitter (otherwise stddev=0 and the
	// implementation correctly skips). EC2 stays in-distribution, S3 spikes.
	jitter := []float64{0, 2, -1, 1, -2, 1, 0}
	var rows []storage.CostRow
	for i := 1; i <= 7; i++ {
		date := "2026-01-0" + string(rune('0'+i))
		rows = append(rows,
			dailyRow(date, "EC2", 100+jitter[i-1]),
			dailyRow(date, "S3", 50+jitter[i-1]),
		)
	}
	rows = append(rows,
		dailyRow("2026-01-08", "EC2", 101), // EC2 in-distribution
		dailyRow("2026-01-08", "S3", 500),  // S3 spike
	)
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 anomaly (S3), got %d: %+v", len(got), got)
	}
	if got[0].Service != "S3" {
		t.Errorf("expected S3 anomaly, got %s", got[0].Service)
	}
}

func TestDetect_IgnoresZeroStddevWindow(t *testing.T) {
	// All baseline days identical → stddev = 0 → no sigma can be computed → skip.
	// (This is the safety branch in the implementation.)
	rows := []storage.CostRow{
		dailyRow("2026-01-01", "EC2", 100),
		dailyRow("2026-01-02", "EC2", 100),
		dailyRow("2026-01-03", "EC2", 100),
		dailyRow("2026-01-04", "EC2", 100),
		dailyRow("2026-01-05", "EC2", 100),
		dailyRow("2026-01-06", "EC2", 100),
		dailyRow("2026-01-07", "EC2", 100),
		dailyRow("2026-01-08", "EC2", 200),
	}
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 1})
	if len(got) != 0 {
		t.Fatalf("expected no anomalies when baseline stddev=0, got %d", len(got))
	}
}

func TestDetect_DescendingDeltaOrder(t *testing.T) {
	// Two services both spiking; output must be sorted by delta descending.
	// Both baselines need variance for stddev > 0.
	jitter := []float64{0, 2, -1, 1, -2, 1, 0}
	var rows []storage.CostRow
	for i := 1; i <= 7; i++ {
		date := "2026-01-0" + string(rune('0'+i))
		rows = append(rows,
			storage.CostRow{Date: date, Service: "EC2", UsageType: "X", Amount: 100 + jitter[i-1], Currency: "USD"},
			storage.CostRow{Date: date, Service: "S3", UsageType: "X", Amount: 50 + jitter[i-1], Currency: "USD"},
		)
	}
	rows = append(rows,
		storage.CostRow{Date: "2026-01-08", Service: "EC2", UsageType: "X", Amount: 600, Currency: "USD"}, // delta ~$500
		storage.CostRow{Date: "2026-01-08", Service: "S3", UsageType: "X", Amount: 200, Currency: "USD"},  // delta ~$150
	)
	got := Detect(rows, Config{ThresholdSigma: 2.0, BaselineDays: 7, MinDeltaUSD: 50})
	if len(got) < 2 {
		t.Fatalf("expected 2 anomalies, got %d: %+v", len(got), got)
	}
	if got[0].Delta < got[1].Delta {
		t.Errorf("anomalies not sorted by delta desc: %v", got)
	}
}
