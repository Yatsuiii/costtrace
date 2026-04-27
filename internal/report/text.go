package report

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

var (
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleService = lipgloss.NewStyle().Bold(true)
	styleUp      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

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
