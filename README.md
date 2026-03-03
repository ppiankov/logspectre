# logspectre

[![CI](https://github.com/ppiankov/logspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/logspectre/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppiankov/logspectre)](https://goreportcard.com/report/github.com/ppiankov/logspectre)

**logspectre** — Cloud log storage waste auditor. Part of [SpectreHub](https://github.com/ppiankov/spectrehub).

## What it is

- Scans CloudWatch Logs, GCP Cloud Logging, and Azure Monitor for waste
- Detects stale log groups, missing retention policies, and unread logs
- Checks ingestion activity, consumer subscriptions, and storage metrics
- Estimates monthly cost per finding
- Outputs text, JSON, SARIF, and SpectreHub formats

## What it is NOT

- Not a log analysis tool — audits infrastructure, not content
- Not a monitoring tool — point-in-time scanner
- Not a remediation tool — reports only, never modifies log groups
- Not a log aggregator — does not read, ship, or process log data

## Quick start

### Homebrew

```sh
brew tap ppiankov/tap
brew install logspectre
```

### From source

```sh
git clone https://github.com/ppiankov/logspectre.git
cd logspectre
make build
```

### Usage

```sh
logspectre scan --provider aws --region us-east-1 --format json
```

## CLI commands

| Command | Description |
|---------|-------------|
| `logspectre scan` | Scan cloud log storage for waste |
| `logspectre init` | Generate config file and permissions |
| `logspectre version` | Print version |

## SpectreHub integration

logspectre feeds log storage waste findings into [SpectreHub](https://github.com/ppiankov/spectrehub) for unified visibility across your infrastructure.

```sh
spectrehub collect --tool logspectre
```

## Safety

logspectre operates in **read-only mode**. It inspects and reports — never modifies, deletes, or alters your log groups.

## License

MIT — see [LICENSE](LICENSE).

---

Built by [Obsta Labs](https://github.com/ppiankov)
