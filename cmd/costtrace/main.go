package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	awsint "github.com/Yatsuiii/costtrace/internal/aws"
	"github.com/Yatsuiii/costtrace/internal/config"
	"github.com/Yatsuiii/costtrace/internal/correlate"
	"github.com/Yatsuiii/costtrace/internal/detect"
	ghint "github.com/Yatsuiii/costtrace/internal/github"
	"github.com/Yatsuiii/costtrace/internal/report"
	"github.com/Yatsuiii/costtrace/internal/storage"
)

const configPath = "config.toml"
const dbPath = "costtrace.db"

func main() {
	root := &cobra.Command{
		Use:   "costtrace",
		Short: "Deploy-to-cost causal attribution",
	}
	root.AddCommand(cmdInit(), cmdPull(), cmdAnomalies(), cmdAnalyze(), cmdExplain(), cmdReport())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func cmdInit() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive setup → config.toml",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Defaults()
			r := bufio.NewReader(os.Stdin)
			ask := func(prompt, def string) string {
				fmt.Printf("%s [%s]: ", prompt, def)
				line, _ := r.ReadString('\n')
				line = strings.TrimSpace(line)
				if line == "" {
					return def
				}
				return line
			}
			cfg.AWS.Profile = ask("AWS profile", cfg.AWS.Profile)
			cfg.AWS.Region = ask("AWS region", cfg.AWS.Region)
			cfg.AWS.AccountID = ask("AWS account ID", cfg.AWS.AccountID)
			cfg.GitHub.Repo = ask("GitHub repo (owner/name)", cfg.GitHub.Repo)
			cfg.GitHub.TokenEnv = ask("GitHub token env var", cfg.GitHub.TokenEnv)
			cfg.GitHub.DeployWorkflowPattern = ask("Deploy workflow name pattern (regex)", cfg.GitHub.DeployWorkflowPattern)
			if err := config.Write(configPath, cfg); err != nil {
				return err
			}
			fmt.Println("Wrote config.toml")
			return nil
		},
	}
}

func cmdPull() *cobra.Command {
	var sinceDays int
	var skipGitHub, skipCloudTrail bool
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch and cache cost, deploy, and CloudTrail data",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config (run `costtrace init` first): %w", err)
			}
			db, err := storage.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			ctx := context.Background()

			awsCfg, err := awsint.LoadConfig(ctx, cfg.AWS.Profile, cfg.AWS.Region)
			if err != nil {
				return err
			}

			// Cost Explorer
			ce := awsint.NewCEClient(awsCfg)
			cursor, _ := db.GetCursor("cost_last_date")
			start := time.Now().UTC().AddDate(0, 0, -sinceDays)
			if cursor != "" {
				if t, err2 := time.Parse("2006-01-02", cursor); err2 == nil && t.After(start) {
					start = t.AddDate(0, 0, 1)
				}
			}
			end := time.Now().UTC()
			fmt.Printf("Pulling costs %s → %s...\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
			costRows, err := ce.FetchCosts(ctx, start, end)
			if err != nil {
				return err
			}
			if err := db.UpsertCost(costRows); err != nil {
				return err
			}
			_ = db.SetCursor("cost_last_date", end.AddDate(0, 0, -1).Format("2006-01-02"))
			fmt.Printf("Stored %d cost rows.\n", len(costRows))

			// GitHub Actions
			var deploys []storage.DeployRow
			if !skipGitHub && cfg.GitHub.Repo != "" {
				token := os.Getenv(cfg.GitHub.TokenEnv)
				ghClient, err := ghint.NewClient(token, cfg.GitHub.Repo, cfg.GitHub.DeployWorkflowPattern)
				if err != nil {
					return fmt.Errorf("github client: %w", err)
				}
				since := time.Now().UTC().AddDate(0, 0, -sinceDays)
				fmt.Printf("Pulling deploy runs from %s since %s...\n", cfg.GitHub.Repo, since.Format("2006-01-02"))
				deploys, err = ghClient.FetchRuns(ctx, since)
				if err != nil {
					return fmt.Errorf("fetch github runs: %w", err)
				}
				if err := db.UpsertDeploys(deploys); err != nil {
					return err
				}
				fmt.Printf("Stored %d deploy events.\n", len(deploys))
			}

			// CloudTrail — only for deploy windows, not all events
			if !skipCloudTrail && len(deploys) > 0 {
				ct := awsint.NewCTClient(awsCfg)
				windowHours := cfg.Detection.CorrelationWindowHours
				if windowHours == 0 {
					windowHours = 4
				}
				fmt.Printf("Fetching CloudTrail events for %d deploy windows...\n", len(deploys))
				ctRows, err := ct.FetchEventsForDeploys(ctx, deploys, windowHours)
				if err != nil {
					return fmt.Errorf("cloudtrail: %w", err)
				}
				if err := db.UpsertCloudTrail(ctRows); err != nil {
					return err
				}
				fmt.Printf("Stored %d CloudTrail events.\n", len(ctRows))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&sinceDays, "since", 30, "days of history to pull")
	cmd.Flags().BoolVar(&skipGitHub, "no-github", false, "skip GitHub pull")
	cmd.Flags().BoolVar(&skipCloudTrail, "no-cloudtrail", false, "skip CloudTrail pull")
	return cmd
}

func cmdAnomalies() *cobra.Command {
	var days int
	var format string
	cmd := &cobra.Command{
		Use:   "anomalies",
		Short: "List cost anomalies",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config (run `costtrace init` first): %w", err)
			}
			db, err := storage.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := loadAnomalies(db, cfg, days)
			if err != nil {
				return err
			}
			if format == "markdown" {
				report.AnomaliesMarkdown(os.Stdout, rows)
			} else {
				report.Anomalies(os.Stdout, rows)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "window to scan for anomalies")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text | markdown")
	return cmd
}

func cmdReport() *cobra.Command {
	var days int
	var format string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Write a polished report",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config (run `costtrace init` first): %w", err)
			}
			db, err := storage.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			since := time.Now().UTC().AddDate(0, 0, -days)
			anomalyRows, err := db.Anomalies(since)
			if err != nil {
				return err
			}

			deploys, err := db.DeploysBetween(since.AddDate(0, 0, -1), time.Now().UTC())
			if err != nil {
				return err
			}
			var deployIDs []string
			for _, d := range deploys {
				deployIDs = append(deployIDs, d.ID)
			}
			ctRows, err := db.CloudTrailForDeployList(deployIDs)
			if err != nil {
				return err
			}
			ctByDeploy := map[string][]storage.CloudTrailRow{}
			for _, r := range ctRows {
				ctByDeploy[r.DeployID] = append(ctByDeploy[r.DeployID], r)
			}

			windowHours := cfg.Detection.CorrelationWindowHours
			if windowHours == 0 {
				windowHours = 4
			}
			results := correlate.Match(anomalyRows, deploys, windowHours, ctByDeploy)

			if format == "markdown" {
				report.AnalyzeMarkdown(os.Stdout, results, time.Now().UTC())
			} else {
				report.Analyze(os.Stdout, results)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "window to report on")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text | markdown")
	return cmd
}

func cmdAnalyze() *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Anomalies + deploy correlations with CloudTrail lineage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config (run `costtrace init` first): %w", err)
			}
			db, err := storage.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			anomalyRows, err := loadAnomalies(db, cfg, days)
			if err != nil {
				return err
			}
			if len(anomalyRows) == 0 {
				fmt.Println("No anomalies detected.")
				return nil
			}
			if err := db.InsertAnomalies(anomalyRows); err != nil {
				return err
			}
			since := time.Now().UTC().AddDate(0, 0, -days)
			anomalyRows, err = db.Anomalies(since)
			if err != nil {
				return err
			}

			deploys, err := db.DeploysBetween(since.AddDate(0, 0, -1), time.Now().UTC())
			if err != nil {
				return err
			}

			// load CloudTrail keyed by deploy ID
			var deployIDs []string
			for _, d := range deploys {
				deployIDs = append(deployIDs, d.ID)
			}
			ctRows, err := db.CloudTrailForDeployList(deployIDs)
			if err != nil {
				return err
			}
			ctByDeploy := map[string][]storage.CloudTrailRow{}
			for _, r := range ctRows {
				ctByDeploy[r.DeployID] = append(ctByDeploy[r.DeployID], r)
			}

			windowHours := cfg.Detection.CorrelationWindowHours
			if windowHours == 0 {
				windowHours = 4
			}
			results := correlate.Match(anomalyRows, deploys, windowHours, ctByDeploy)

			var corrRows []storage.CorrelationRow
			for _, res := range results {
				for _, c := range res.Candidates {
					corrRows = append(corrRows, storage.CorrelationRow{
						AnomalyID:  res.Anomaly.ID,
						DeployID:   c.Deploy.ID,
						Confidence: c.Confidence,
						Evidence:   c.Evidence,
					})
				}
			}
			if len(corrRows) > 0 {
				_ = db.UpsertCorrelations(corrRows)
			}

			report.Analyze(os.Stdout, results)
			fmt.Fprintf(os.Stdout, "ACE_SUMMARY: %s\n", aceSummary(days, len(anomalyRows), len(corrRows)))
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "window to analyze")
	return cmd
}

func cmdExplain() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <anomaly-id>",
		Short: "Deep-dive a single anomaly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("anomaly-id must be a number: %w", err)
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config (run `costtrace init` first): %w", err)
			}
			db, err := storage.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			// reload all anomalies and find the one with matching ID
			anomalyRows, _ := db.Anomalies(time.Time{})
			var target *storage.AnomalyRow
			for i, a := range anomalyRows {
				if a.ID == id {
					target = &anomalyRows[i]
					break
				}
			}
			if target == nil {
				return fmt.Errorf("anomaly %d not found — run `costtrace analyze` first", id)
			}

			correlations, err := db.CorrelationsForAnomaly(id)
			if err != nil {
				return err
			}

			// enrich with deploy rows
			var deployIDs []string
			for _, c := range correlations {
				deployIDs = append(deployIDs, c.DeployID)
			}
			deploys, err := db.DeploysBetween(time.Time{}, time.Now().UTC())
			if err != nil {
				return err
			}
			deployMap := map[string]storage.DeployRow{}
			for _, d := range deploys {
				deployMap[d.ID] = d
			}

			ctRows, err := db.CloudTrailForDeployList(deployIDs)
			if err != nil {
				return err
			}
			ctByDeploy := map[string][]storage.CloudTrailRow{}
			for _, r := range ctRows {
				ctByDeploy[r.DeployID] = append(ctByDeploy[r.DeployID], r)
			}

			windowHours := cfg.Detection.CorrelationWindowHours
			if windowHours == 0 {
				windowHours = 4
			}
			var candidates []correlate.Candidate
			for _, c := range correlations {
				d := deployMap[c.DeployID]
				candidates = append(candidates, correlate.Candidate{
					Deploy:     d,
					Confidence: c.Confidence,
					Evidence:   c.Evidence,
				})
			}
			result := correlate.Result{Anomaly: *target, Candidates: candidates}
			report.Explain(os.Stdout, result, ctByDeploy)
			return nil
		},
	}
}

func loadAnomalies(db *storage.DB, cfg *config.Config, days int) ([]storage.AnomalyRow, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	costRows, err := db.CostSince(since.AddDate(0, 0, -cfg.Detection.BaselineDays))
	if err != nil {
		return nil, err
	}
	if len(costRows) == 0 {
		fmt.Println("No cost data found. Run `costtrace pull` first.")
		return nil, nil
	}
	dcfg := detect.Config{
		ThresholdSigma: cfg.Detection.ThresholdSigma,
		BaselineDays:   cfg.Detection.BaselineDays,
		MinDeltaUSD:    cfg.Detection.MinDeltaUSD,
	}
	anomalies := detect.Detect(costRows, dcfg)
	cutoff := since.Format("2006-01-02")
	var filtered []storage.AnomalyRow
	for _, a := range detect.ToRows(anomalies) {
		if a.Date >= cutoff {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

func aceSummary(windowDays, anomalies, correlations int) string {
	b, _ := json.Marshal(map[string]any{
		"v":            1,
		"command":      "analyze",
		"anomalies":    anomalies,
		"correlations": correlations,
		"window_days":  windowDays,
	})
	return string(b)
}
