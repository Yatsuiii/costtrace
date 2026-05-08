# costtrace

[![ci](https://github.com/Yatsuiii/costtrace/actions/workflows/ci.yml/badge.svg)](https://github.com/Yatsuiii/costtrace/actions/workflows/ci.yml)

Deploy-to-cost causal attribution for AWS.

> Which PR caused this $4k/day spike?

`costtrace` joins AWS cost anomalies to the GitHub Actions deploys that caused them — using CloudTrail resource creation events, not tag hygiene.

```
$ costtrace analyze --days 14

Anomaly  EC2  2026-04-21  (3.2σ)
  Actual: $4,103.00  Baseline: $256.00  +$3,847.00

  Candidates:
    [0.85] deploy-prod · PR #482 · 2026-04-21 14:35 UTC
      time_window: deploy completed within 4h of anomaly
      [resource_creation] 24 resource(s) created matching service "EC2" (i-0a1b2c, i-0d3e4f...)
      [principal_match] CloudTrail principal "github-actions-deploy-role" looks like CI/CD

    [0.05] update-docs · 2026-04-21 14:50 UTC
      time_window: deploy completed within 4h of anomaly (time-window only)

ACE_SUMMARY: {"anomalies":1,"command":"analyze","correlations":1,"v":1,"window_days":14}
```

## How it works

1. **Pull** — fetches 30 days of daily cost data from AWS Cost Explorer, deploy runs from GitHub Actions, and CloudTrail write events scoped to each deploy window.
2. **Detect** — flags cost spikes using a 7-day rolling baseline + standard deviation threshold per service.
3. **Correlate** — matches anomalies to deploys that landed within the correlation window, then scores confidence based on CloudTrail evidence.

The join uses CloudTrail resource creation events to link *what was deployed* to *what appeared in the cost delta* — no tagging required.

| Evidence | Confidence weight |
|---|---|
| Resource created in deploy window AND matches anomalous service | +0.60 |
| CloudTrail principal matches a CI/CD role or user agent | +0.20 |
| Time-window only (no resource creation evidence) | 0.05 |

Confidence is additive, capped at 1.0. A deploy that just happened to be in the window scores 0.05 — explicitly low.

## Install

```bash
git clone https://github.com/Yatsuiii/costtrace
cd costtrace
go build -o costtrace ./cmd/costtrace
```

Requires Go 1.21+. No CGo, no external binaries.

## Setup

### AWS permissions

The IAM user or role running `costtrace` needs:

```json
{
  "Effect": "Allow",
  "Action": [
    "ce:GetCostAndUsage",
    "cloudtrail:LookupEvents"
  ],
  "Resource": "*"
}
```

### GitHub token

A classic token with `repo` scope (read-only on Actions) or a fine-grained token with `actions:read`.

```bash
export GITHUB_TOKEN=ghp_...
```

### Config

```bash
./costtrace init
```

Writes `config.toml`. See `examples/basic-config.toml` for all options.

## Usage

```bash
# fetch everything (cost + deploys + cloudtrail)
./costtrace pull --since 30

# list anomalies only
./costtrace anomalies --days 30

# full causal analysis
./costtrace analyze --days 30

# deep-dive one anomaly (get ID from analyze output)
./costtrace explain <anomaly-id>

# markdown report for a PR comment or wiki
./costtrace report --format markdown --days 14
```

## Limitations

**These are real, not fine print.**

1. **24–48h billing lag.** AWS Cost Explorer data is delayed. This tool is retrospective — it tells you what caused a spike *after* the bill updates. It is not a real-time alert.

2. **Correlation ≠ causation.** A deploy that lands in the time window is a candidate, not a proven cause. Read confidence scores carefully: 0.05 means "it was nearby," not "it did it." Only resource creation evidence pushes confidence meaningfully above baseline.

3. **Single AWS account only.** No org-level aggregation. Multi-account support requires CUR + Athena, which is out of scope for the MVP.

4. **CloudTrail volume.** Write events in active accounts can be thousands per minute. `costtrace` pulls only within deploy windows (not all of CloudTrail) and filters to ~15 resource-creation event types. Busy accounts may still hit CloudTrail's LookupEvents rate limits — the client skips throttled pages rather than failing.

5. **Tag-free design.** If your team has perfect tag hygiene, Vantage does this better. `costtrace` is for teams that don't.

6. **AWS only.** No Azure, GCP, or multi-cloud. The join is CloudTrail-specific.

## How it differs from other tools

| Tool | What it does | What it misses |
|---|---|---|
| AWS Cost Explorer | Time-series cost slices | No deploy linkage |
| Cloudability / CloudHealth | Tag-based showback | Tag-dependent, no causal trace |
| Infracost | Pre-merge PR cost estimates | Predictive, not post-deploy actuals |
| Vantage | Reports + tag correlation | Tag-dependent, no CloudTrail join |
| **costtrace** | **Deploy → CloudTrail → cost causal chain** | AWS only, single account |

## License

MIT
