# LogSpectre

[![ANCC](https://img.shields.io/badge/ANCC-compliant-brightgreen)](https://ancc.dev)
[![CI](https://github.com/ppiankov/logspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/logspectre/actions/workflows/ci.yml)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Cloud log storage waste auditor. Finds stale log groups, missing retention policies, and unread logs across CloudWatch, Cloud Logging, and Azure Monitor.

Part of the [Spectre family](https://spectrehub.dev) of infrastructure cleanup tools.

## What it is

LogSpectre scans your cloud logging infrastructure for log groups, buckets, and workspaces that are accumulating cost without delivering value. It checks ingestion activity, retention policies, consumer subscriptions, and storage metrics to identify waste. Each finding includes an estimated monthly cost so you can prioritize cleanup by dollar impact.

## What it is NOT

- Not a log analysis tool. LogSpectre audits log infrastructure, not log content.
- Not a monitoring tool. It is a point-in-time scanner, not a daemon or alerting system.
- Not a remediation tool. It reports waste and lets you decide what to do.
- Not a log aggregator. It does not read, ship, or process log data.
- Not a billing replacement. Cost estimates use published per-GB rates, not your actual negotiated pricing.

## Philosophy

*Principiis obsta* — resist the beginnings.

Log storage is the most common source of silent cloud waste. A log group created for a service that was decommissioned a year ago still accrues storage charges. A log group with no retention policy grows forever. A log group with high ingestion but no metric filters or subscriptions is writing logs nobody reads. LogSpectre surfaces these conditions early so they can be fixed before costs compound.

The tool presents evidence and lets humans decide. It does not delete log groups, does not modify retention policies, and does not use ML where deterministic checks suffice.

## Installation

```bash
# Homebrew
brew install ppiankov/tap/logspectre

# From source
git clone https://github.com/ppiankov/logspectre.git
cd logspectre && make build
```

## Quick start

```bash
# Scan all providers (AWS + GCP + Azure)
logspectre scan --platform all

# Scan AWS CloudWatch only
logspectre scan --platform aws

# Scan GCP Cloud Logging
logspectre scan --platform gcp --project my-project

# Scan Azure Monitor
logspectre scan --platform azure --subscription xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

# Custom idle threshold
logspectre scan --platform aws --idle-days 30

# Filter by minimum cost
logspectre scan --platform aws --min-cost 10.0

# Scan specific regions
logspectre scan --platform aws --region us-east-1,eu-west-1

# JSON output for automation
logspectre scan --platform aws --format json

# Generate sample config
logspectre init
```

Requires valid cloud credentials for each platform being scanned.

## What it audits

| Finding | Severity | Signal |
|---------|----------|--------|
| `STALE` | high | No log activity (zero ingestion events) over lookback period |
| `NO_RETENTION` | high | No expiration policy — logs stored indefinitely at growing cost |
| `EMPTY` | medium | Log group/bucket stores zero bytes |
| `HIGH_INGESTION` | medium | Daily ingestion exceeds threshold (default 1 GiB/day) |
| `UNREAD` | medium | Logs written but never consumed (no metric filters or subscriptions) |
| `STALE_SINK` | low | GCP log sink exported zero bytes over lookback period |

## Cost model

LogSpectre estimates monthly cost per finding using published rates:

| Provider | Ingestion Rate | Storage Rate |
|----------|---------------|--------------|
| AWS CloudWatch Logs | $0.50/GB ingested | $0.03/GB/month stored |
| GCP Cloud Logging | $0.50/GiB ingested | included |
| Azure Log Analytics | $2.76/GB ingested | included |

Estimates are directional. Your actual costs depend on committed-use discounts, free tiers, and regional pricing.

## Usage

```bash
logspectre scan [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-p, --platform` | `all` | Platform: `all`, `aws`, `gcp`, `azure` |
| `-f, --format` | `text` | Output format: `text`, `json` |
| `-r, --region` | all | Comma-separated region filter |
| `-d, --idle-days` | `90` | Lookback window for activity detection |
| `-c, --min-cost` | `0.0` | Minimum monthly cost to report ($) |
| `--project` | | GCP project ID (required for `--platform gcp`) |
| `--subscription` | | Azure subscription ID (required for `--platform azure`) |

**Other commands:**

| Command | Description |
|---------|-------------|
| `logspectre init` | Generate `.logspectre.yaml` config file |
| `logspectre version` | Print version, commit, and build date |

## Authentication

| Provider | Method | Details |
|----------|--------|---------|
| AWS CloudWatch | Default credential chain | IAM role, `~/.aws/credentials`, `AWS_*` env vars |
| GCP Cloud Logging | Application Default Credentials | `gcloud auth`, `GOOGLE_APPLICATION_CREDENTIALS` |
| Azure Log Analytics | DefaultAzureCredential | `az login`, env vars, managed identity |

## Output formats

**Text** (default): Human-readable table grouped by provider, sorted by cost descending.

**JSON** (`--format json`): `spectre/v1` envelope with findings and summary:
```json
{
  "schema": "spectre/v1",
  "tool": "logspectre",
  "target": { "type": "cloud-account", "name": "sha256:..." },
  "findings": [...],
  "summary": {
    "total_findings": 12,
    "total_monthly_cost": 145.50
  }
}
```

## Architecture

```
logspectre/
├── cmd/logspectre/main.go          # Entry point (LDFLAGS version injection)
├── internal/
│   ├── commands/                   # Cobra CLI: scan, init, version
│   ├── analyzer/                   # Finding detection + cost calculation
│   │   ├── analyzer.go            # Platform-specific analyzers (AWS, GCP, Azure)
│   │   ├── types.go               # Finding types and Provider enum
│   │   └── cost.go                # Per-provider cost formulas
│   ├── aws/                        # CloudWatch Logs scanner
│   │   └── cloudwatch.go          # Log groups, retention, metrics, subscriptions
│   ├── gcp/                        # Cloud Logging scanner
│   │   └── logging.go             # Log buckets, sinks, export metrics
│   ├── azure/                      # Azure Monitor scanner
│   │   └── monitor.go             # Log Analytics workspaces, heartbeat, quota
│   ├── config/                     # YAML config loader
│   ├── report/                     # Text, JSON reporters
│   └── logging/                    # Structured logging
├── Makefile
└── go.mod
```

Key design decisions:

- **Three-provider parity.** AWS, GCP, and Azure scanners implement the same detection logic adapted to each platform's APIs.
- **Cost centralization.** All pricing formulas live in `cost.go` — no hardcoded rates in scanners.
- **Interface-based testing.** All cloud API calls go through interfaces, enabling full unit testing without live credentials.
- **Cost-sorted output.** Text reporter sorts findings by estimated cost descending so the biggest wins appear first.
- **Read-only.** LogSpectre never modifies log groups, retention policies, or subscriptions.

## Project status

**Status: Beta** · **v0.1.0** · Pre-1.0

| Milestone | Status |
|-----------|--------|
| 3 cloud providers (CloudWatch, Cloud Logging, Azure Monitor) | Complete |
| 6 finding types (stale, no retention, empty, high ingestion, unread, stale sink) | Complete |
| Per-finding cost estimation | Complete |
| 2 output formats (text, JSON) | Complete |
| Config file + init command | Complete |
| CI pipeline (test/lint/build) | Complete |
| Homebrew distribution | Complete |
| SARIF output | Planned |
| SpectreHub format | Planned |
| Datadog/Splunk provider support | Planned |
| v1.0 release | Planned |

Pre-1.0: CLI flags and config schemas may change between minor versions. JSON output structure (`spectre/v1`) is stable.

## Known limitations

- **Approximate pricing.** Cost estimates use published on-demand rates. Committed-use discounts, free tiers, and regional pricing differences are not reflected.
- **CloudWatch metric lag.** CloudWatch metrics may take up to 15 minutes to appear. Very recently created log groups may not have enough data.
- **GCP sink metrics.** Sink activity is measured by export byte count from Cloud Monitoring. Sinks that export to destinations without monitoring integration may appear stale.
- **Azure heartbeat proxy.** Azure workspace activity is inferred from Heartbeat metrics, which may not reflect all log sources.
- **No log content analysis.** LogSpectre checks infrastructure metrics only. It does not read or analyze log content.
- **Single account/project/subscription.** Scans one cloud account at a time per provider.

## License

MIT License — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Issues and pull requests welcome.

Part of the [Spectre family](https://spectrehub.dev).
