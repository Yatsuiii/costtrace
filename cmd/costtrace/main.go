package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	awsint "github.com/Yatsuiii/costtrace/internal/aws"
	"github.com/Yatsuiii/costtrace/internal/config"
	"github.com/Yatsuiii/costtrace/internal/detect"
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
	root.AddCommand(cmdInit(), cmdPull(), cmdAnomalies())
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
			cfg.Detection.ThresholdSigma = 2.0
			cfg.Detection.BaselineDays = 7
			cfg.Detection.MinDeltaUSD = 50

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
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch and cache cost data",
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
			ce := awsint.NewCEClient(awsCfg)

			// use cursor for incremental pull
			cursor, _ := db.GetCursor("cost_last_date")
			start := time.Now().UTC().AddDate(0, 0, -sinceDays)
			if cursor != "" {
				if t, err := time.Parse("2006-01-02", cursor); err == nil && t.After(start) {
					start = t.AddDate(0, 0, 1)
				}
			}
			end := time.Now().UTC()

			fmt.Printf("Pulling costs %s → %s...\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
			rows, err := ce.FetchCosts(ctx, start, end)
			if err != nil {
				return err
			}
			if err := db.UpsertCost(rows); err != nil {
				return err
			}
			_ = db.SetCursor("cost_last_date", end.AddDate(0, 0, -1).Format("2006-01-02"))
			fmt.Printf("Stored %d cost rows.\n", len(rows))
			return nil
		},
	}
	cmd.Flags().IntVar(&sinceDays, "since", 30, "days of history to pull")
	return cmd
}

func cmdAnomalies() *cobra.Command {
	var days int
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

			since := time.Now().UTC().AddDate(0, 0, -days)
			costRows, err := db.CostSince(since.AddDate(0, 0, -cfg.Detection.BaselineDays))
			if err != nil {
				return err
			}
			if len(costRows) == 0 {
				fmt.Println("No cost data found. Run `costtrace pull` first.")
				return nil
			}

			dcfg := detect.Config{
				ThresholdSigma: cfg.Detection.ThresholdSigma,
				BaselineDays:   cfg.Detection.BaselineDays,
				MinDeltaUSD:    cfg.Detection.MinDeltaUSD,
			}
			anomalies := detect.Detect(costRows, dcfg)

			// filter to requested window
			cutoff := since.Format("2006-01-02")
			var filtered []storage.AnomalyRow
			for _, a := range detect.ToRows(anomalies) {
				if a.Date >= cutoff {
					filtered = append(filtered, a)
				}
			}

			report.Anomalies(os.Stdout, filtered)
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "window to scan for anomalies")
	return cmd
}
