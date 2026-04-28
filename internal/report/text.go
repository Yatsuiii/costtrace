package report

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/Yatsuiii/costtrace/internal/correlate"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

var (
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleService = lipgloss.NewStyle().Bold(true)
	styleUp      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func Analyze(w io.Writer, results []correlate.Result) {
	for _, res := range results {
		a := res.Anomaly
		fmt.Fprintf(w, "%s  %s  %s  (%.1fσ)\n",
			styleHeader.Render("Anomaly"),
			styleService.Render(a.Service),
			styleDim.Render(a.Date),
			a.Sigma,
		)
		fmt.Fprintf(w, "  Actual: %s  Baseline: $%.2f  %s\n",
			styleUp.Render(fmt.Sprintf("$%.2f", a.ActualAmount)),
			a.BaselineAmount,
			styleUp.Render(fmt.Sprintf("+$%.2f", a.Delta)),
		)
		if len(res.Candidates) == 0 {
			fmt.Fprintln(w, styleDim.Render("  No deploys found in correlation window."))
		} else {
			fmt.Fprintln(w, "  Candidates:")
			for _, c := range res.Candidates {
				d := c.Deploy
				pr := ""
				if d.PRNumber != nil {
					pr = fmt.Sprintf(" PR #%d", *d.PRNumber)
				}
				fmt.Fprintf(w, "    [%.2f] %s%s · %s\n",
					c.Confidence,
					styleService.Render(d.Title),
					pr,
					styleDim.Render(d.CompletedAt.Format("2006-01-02 15:04 UTC")),
				)
				fmt.Fprintf(w, "      %s\n", styleDim.Render(c.Evidence))
			}
		}
		fmt.Fprintln(w)
	}
}

func Explain(w io.Writer, result correlate.Result, ctByDeploy map[string][]storage.CloudTrailRow) {
	a := result.Anomaly
	fmt.Fprintln(w, styleHeader.Render("── Anomaly Detail ──────────────────────────────"))
	fmt.Fprintf(w, "  ID:       %d\n", a.ID)
	fmt.Fprintf(w, "  Service:  %s\n", styleService.Render(a.Service))
	fmt.Fprintf(w, "  Date:     %s\n", a.Date)
	fmt.Fprintf(w, "  Actual:   %s\n", styleUp.Render(fmt.Sprintf("$%.2f", a.ActualAmount)))
	fmt.Fprintf(w, "  Baseline: $%.2f  (7-day rolling mean)\n", a.BaselineAmount)
	fmt.Fprintf(w, "  Delta:    %s  (%.1fσ)\n", styleUp.Render(fmt.Sprintf("+$%.2f", a.Delta)), a.Sigma)
	fmt.Fprintln(w)

	if len(result.Candidates) == 0 {
		fmt.Fprintln(w, styleDim.Render("  No deploy correlations found."))
		return
	}

	fmt.Fprintln(w, styleHeader.Render("── Correlated Deploys ──────────────────────────"))
	for _, c := range result.Candidates {
		d := c.Deploy
		pr := ""
		if d.PRNumber != nil {
			pr = fmt.Sprintf(" · PR #%d", *d.PRNumber)
		}
		fmt.Fprintf(w, "\n  [conf %.2f] %s%s\n", c.Confidence, styleService.Render(d.Title), pr)
		fmt.Fprintf(w, "    Repo:    %s\n", d.Repo)
		fmt.Fprintf(w, "    Branch:  %s\n", d.Branch)
		fmt.Fprintf(w, "    Commit:  %s\n", d.CommitSHA[:min(12, len(d.CommitSHA))])
		fmt.Fprintf(w, "    Started: %s\n", d.StartedAt.UTC().Format("2006-01-02 15:04 UTC"))
		fmt.Fprintf(w, "    Done:    %s\n", d.CompletedAt.UTC().Format("2006-01-02 15:04 UTC"))
		fmt.Fprintf(w, "    %s\n", styleDim.Render(c.Evidence))

		events := ctByDeploy[d.ID]
		if len(events) > 0 {
			fmt.Fprintf(w, "\n    CloudTrail events (%d):\n", len(events))
			shown := 0
			for _, e := range events {
				if shown >= 10 {
					fmt.Fprintf(w, "      %s\n", styleDim.Render(fmt.Sprintf("... and %d more", len(events)-shown)))
					break
				}
				fmt.Fprintf(w, "      %s  %-30s  %s\n",
					styleDim.Render(e.EventTime.UTC().Format("15:04:05")),
					e.EventName,
					styleDim.Render(e.ResourceID),
				)
				shown++
			}
		}
	}
	fmt.Fprintln(w)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Anomalies(w io.Writer, rows []storage.AnomalyRow) {
	if len(rows) == 0 {
		fmt.Fprintln(w, styleDim.Render("No anomalies detected."))
		return
	}
	fmt.Fprintln(w, styleHeader.Render(fmt.Sprintf("Found %d anomaly(s):", len(rows))))
	fmt.Fprintln(w)
	for _, r := range rows {
		fmt.Fprintf(w, "  %s  %s\n",
			styleService.Render(r.Service),
			styleDim.Render(r.Date),
		)
		fmt.Fprintf(w, "    Actual: %s  Baseline: $%.2f  %s  (%.1fσ)\n",
			styleUp.Render(fmt.Sprintf("$%.2f", r.ActualAmount)),
			r.BaselineAmount,
			styleUp.Render(fmt.Sprintf("+$%.2f", r.Delta)),
			r.Sigma,
		)
		fmt.Fprintln(w)
	}
}
