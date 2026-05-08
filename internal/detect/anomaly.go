package detect

import (
	"math"
	"sort"
	"time"

	"github.com/Yatsuiii/costtrace/internal/storage"
)

type Anomaly struct {
	Service        string
	Date           string
	BaselineAmount float64
	ActualAmount   float64
	Delta          float64
	Sigma          float64
}

type Config struct {
	ThresholdSigma float64
	BaselineDays   int
	MinDeltaUSD    float64
}

func Detect(rows []storage.CostRow, cfg Config) []Anomaly {
	// group by service → sorted daily totals
	type point struct {
		date   string
		amount float64
	}
	byService := map[string][]point{}
	for _, r := range rows {
		byService[r.Service] = append(byService[r.Service], point{r.Date, r.Amount})
	}
	// aggregate usage_types per service per day
	dailyTotals := map[string]map[string]float64{} // service → date → total
	for _, r := range rows {
		if dailyTotals[r.Service] == nil {
			dailyTotals[r.Service] = map[string]float64{}
		}
		dailyTotals[r.Service][r.Date] += r.Amount
	}
	_ = byService

	var anomalies []Anomaly
	for service, dates := range dailyTotals {
		type dp struct {
			date string
			amt  float64
		}
		var series []dp
		for d, a := range dates {
			series = append(series, dp{d, a})
		}
		sort.Slice(series, func(i, j int) bool { return series[i].date < series[j].date })

		for i := cfg.BaselineDays; i < len(series); i++ {
			window := series[i-cfg.BaselineDays : i]
			var sum float64
			for _, w := range window {
				sum += w.amt
			}
			mean := sum / float64(len(window))

			var variance float64
			for _, w := range window {
				d := w.amt - mean
				variance += d * d
			}
			stddev := math.Sqrt(variance / float64(len(window)))

			actual := series[i].amt
			delta := actual - mean
			if stddev == 0 || math.Abs(delta) < cfg.MinDeltaUSD {
				continue
			}
			sigma := delta / stddev
			if sigma >= cfg.ThresholdSigma {
				anomalies = append(anomalies, Anomaly{
					Service:        service,
					Date:           series[i].date,
					BaselineAmount: mean,
					ActualAmount:   actual,
					Delta:          delta,
					Sigma:          sigma,
				})
			}
		}
	}
	// sort by delta descending
	sort.Slice(anomalies, func(i, j int) bool { return anomalies[i].Delta > anomalies[j].Delta })
	return anomalies
}

func ToRows(anomalies []Anomaly) []storage.AnomalyRow {
	now := time.Now().UTC().Format(time.RFC3339)
	rows := make([]storage.AnomalyRow, len(anomalies))
	for i, a := range anomalies {
		rows[i] = storage.AnomalyRow{
			DetectedAt:     now,
			Service:        a.Service,
			Date:           a.Date,
			BaselineAmount: a.BaselineAmount,
			ActualAmount:   a.ActualAmount,
			Delta:          a.Delta,
			Sigma:          a.Sigma,
		}
	}
	return rows
}
