package correlate

import (
	"fmt"
	"sort"
	"time"

	"github.com/Yatsuiii/costtrace/internal/storage"
)

type Result struct {
	Anomaly    storage.AnomalyRow
	Candidates []Candidate
}

type Candidate struct {
	Deploy     storage.DeployRow
	Confidence float64
	Evidence   string
}

// Match correlates anomalies to deploy events within windowHours.
// ctByDeploy is optional CloudTrail data keyed by deploy ID; pass nil to skip lineage scoring.
func Match(
	anomalies []storage.AnomalyRow,
	deploys []storage.DeployRow,
	windowHours int,
	ctByDeploy map[string][]storage.CloudTrailRow,
) []Result {
	window := time.Duration(windowHours) * time.Hour
	var results []Result
	for _, a := range anomalies {
		anomalyDate, err := time.Parse("2006-01-02", a.Date)
		if err != nil {
			continue
		}
		windowEnd := anomalyDate.Add(24 * time.Hour)
		windowStart := windowEnd.Add(-window)

		var candidates []Candidate
		for _, d := range deploys {
			if d.CompletedAt.Before(windowStart) || d.StartedAt.After(windowEnd) {
				continue
			}
			evidence := fmt.Sprintf("time_window: deploy completed %s (within %dh of anomaly on %s)",
				d.CompletedAt.UTC().Format("2006-01-02 15:04 UTC"), windowHours, a.Date)
			candidates = append(candidates, Candidate{
				Deploy:     d,
				Confidence: 0.05,
				Evidence:   evidence,
			})
		}

		if ctByDeploy != nil && len(candidates) > 0 {
			candidates = scoreWithLineage(candidates, ctByDeploy, a.Service)
		}

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Confidence > candidates[j].Confidence
		})
		results = append(results, Result{Anomaly: a, Candidates: candidates})
	}
	return results
}
