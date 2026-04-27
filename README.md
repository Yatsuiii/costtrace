# costtrace

Deploy-to-cost causal attribution for AWS.

> Which PR caused this $4k/day spike?

`costtrace` joins your AWS cost anomalies to the GitHub Actions deploys that caused them — using CloudTrail resource creation events, not tag hygiene.

## Status

Pre-MVP. Design phase. See [CLAUDE.md](CLAUDE.md) for the architecture and scope (local only, not in repo).

## Why

AWS Cost Explorer tells you costs went up. Cloudability and Vantage put it on a dashboard. Infracost predicts cost from a PR before merge.

None of them answer the question every engineer actually asks after a bill spike: *which deploy did this?*

`costtrace` does the join.

## How (planned)

```
$ costtrace analyze --days 14

Anomaly · 2026-04-21 (+$3,847 vs 7-day baseline, 3.2σ)
  Service: EC2-Instance + EC2-EBS

  Likely cause (confidence 0.85):
    [a3f9c2] PR #482 "switch to gp3 volumes" — merged 14:32 UTC
      • CloudTrail: 24× CreateVolume (gp3) by ci-deploy-role @ 14:35 UTC
      • Cost delta lines up: gp3 +$2,891, gp2 -$1,204
```

## Roadmap (3 weeks to MVP)

- **Week 1** — Cost Explorer ingestion + statistical anomaly detection
- **Week 2** — GitHub Actions deploy ingestion + time-window correlation
- **Week 3** — CloudTrail resource lineage + confidence scoring + polish

## License

TBD (likely MIT).
