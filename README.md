ISO Socket Stress Test Tool

A high-performance stress testing tool designed to generate ISO8583-style socket traffic against any TCP/TLS server. The tool is optimized for fintech systems that rely on ISO message formats and need predictable, reproducible load patterns.

🚀 Features

TCP / TLS (SSL) socket communication

Parallel workers (goroutine-based)

Configurable Requests Per Second (RPS)

Optional client certificate authentication

Full request/response logging

Round-trip latency measurement

Safe concurrency using channels

Metrics aggregation (success/failure counts, latency, throughput)

This tool is ideal for:

Load testing issuer/acquirer simulators

Benchmarking ISO8583 switch performance

Testing socket-based fintech systems

TLS handshake, certificate validation, and connection reuse behavior

## Running locally

This project now uses the pure-Go `modernc.org/sqlite` driver, so it can run on Windows without enabling CGO or installing a C compiler just to use SQLite/WAL mode.

### Start from the workspace root

```powershell
go run .
```

The application resolves its base path from the current working directory by default. If you want to run it from somewhere else, set `LONGCLAW_BASE_PATH` to the repository root first.

```powershell
$env:LONGCLAW_BASE_PATH = 'C:\Code\longclaw'
go run .
```

Optional overrides:

- `LONGCLAW_DB_PATH`
- `LONGCLAW_HTTP_PORT`
- `LONGCLAW_ISO_PORT`
- `LONGCLAW_SPEC_PATH`
- `LONGCLAW_TEMPLATES_PATH`
- `LONGCLAW_CARDS_PATH`

