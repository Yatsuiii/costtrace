package report

import (
	"fmt"
	"io"
	"time"

	"github.com/Yatsuiii/costtrace/internal/correlate"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

func AnalyzeMarkdown(w io.Writer, results []correlate.Result, generatedAt time.Time) {
	fmt.Fprintf(w, "## costtrace — Cost Anomaly Report\n\n")
	fmt.Fprintf(w, "_Generated %s_\n\n", generatedAt.UTC().Format("2006-01-02 15:04 UTC"))

	if len(results) == 0 {
		fmt.Fprintln(w, "No cost anomalies detected.")
		return
	}

	for _, res := range results {
		a := res.Anomaly
		fmt.Fprintf(w, "### %s · %s\n\n", a.Service, a.Date)
		fmt.Fprintf(w, "| | |\n|---|---|\n")
		fmt.Fprintf(w, "| Actual | **$%.2f** |\n", a.ActualAmount)
		fmt.Fprintf(w, "| 7-day baseline | $%.2f |\n", a.BaselineAmount)
		fmt.Fprintf(w, "| Delta | **+$%.2f** (%.1fσ) |\n\n", a.Delta, a.Sigma)

		if len(res.Candidates) == 0 {
			fmt.Fprintf(w, "_No deploys found in correlation window._\n\n")
			continue
		}

		fmt.Fprintf(w, "**Correlated deploys:**\n\n")
		for _, c := range res.Candidates {
			d := c.Deploy
			pr := ""
			if d.PRNumber != nil {
				pr = fmt.Sprintf(" · PR #%d", *d.PRNumber)
			}
			label := "🔴 Low"
			if c.Confidence >= 0.7 {
				label = "🟢 High"
			} else if c.Confidence >= 0.3 {
				label = "🟡 Medium"
			}
			fmt.Fprintf(w, "- **[conf %.2f %s]** `%s`%s — _%s_\n",
				c.Confidence, label, d.CommitSHA[:min(8, len(d.CommitSHA))], pr,
				d.CompletedAt.UTC().Format("2006-01-02 15:04 UTC"),
			)
			fmt.Fprintf(w, "  - %s (`%s`)\n", d.Title, d.Branch)
			fmt.Fprintf(w, "  - %s\n", c.Evidence)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "---\n_costtrace: deploy-to-cost causal attribution · [github.com/Yatsuiii/costtrace](https://github.com/Yatsuiii/costtrace)_\n")
}

func AnomaliesMarkdown(w io.Writer, rows []storage.AnomalyRow) {
	fmt.Fprintf(w, "## costtrace — Anomalies\n\n")
	if len(rows) == 0 {
		fmt.Fprintln(w, "No anomalies detected.")
		return
	}
	fmt.Fprintln(w, "| Service | Date | Actual | Baseline | Delta | Sigma |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, r := range rows {
		fmt.Fprintf(w, "| %s | %s | $%.2f | $%.2f | +$%.2f | %.1fσ |\n",
			r.Service, r.Date, r.ActualAmount, r.BaselineAmount, r.Delta, r.Sigma)
	}
}
