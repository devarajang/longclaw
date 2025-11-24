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

📑 1. Overview

The stress tester sends ISO-formatted messages at a controlled rate and measures:

Throughput (requests/sec)

Success / failure counts

Network latency per request

Connection and socket health

Server saturation behavior

The design architecture:

|Test Runner| ---Go routines--> | Worker pool | --- channels --> |Metrics Aggregator |


🧠 Key Concepts

1. Worker-per-connection model

Each worker maintains its own TCP or TLS socket. It sends/receives messages and pushes metrics to the aggregator.

2. RPS Scheduling

A central ticker-based scheduler triggers workers at a controlled rate, ensuring exact requests-per-second.

3. Channels for concurrency

A request channel dispatches jobs to workers.

A metrics channel is consumed by a single goroutine ensuring thread-safe aggregation.

⚙️ 2. Configuration

Below is a typical config structure:

{
  "host": "127.0.0.1",
  "port": 5000,
  "use_tls": true,
  "workers": 20,
  "rps": 200,
  "test_duration_secs": 60,
  "iso_message": "30313030...",
  "client_cert_pfx": "client.pfx",
  "client_cert_password": "password"
}

🔌 3. Running the Tool

Example:

./iso-stress-test -config=config.json

Logs and metrics output automatically during and after the run.

🔐 4. TLS / SSL Setup

The tool supports one-way and mutual TLS authentication.

Server Key + Certificate

openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr
openssl x509 -req -in server.csr -signkey server.key -out server.crt -days 365

Client Key + Certificate

openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr
openssl x509 -req -in client.csr -signkey client.key -out client.crt -days 365

Convert Client Certificate to PFX

openssl pkcs12 -export -out client.pfx -inkey client.key -in client.crt -certfile server.crt

This PFX can be loaded by the stress tool for mutual TLS.

📈 5. Metrics Collected

Metric

Description

Latency (ms)

Round-trip time of each request

Success Count

Valid server responses

Failure Count

Timeouts, connection errors, bad responses

Throughput

Actual req/sec achieved

Socket Health

Connect/disconnect tracking

🧵 6. Worker Logic (High-Level)

type Worker struct {
    id       int
    conn     net.Conn
    reqChan  <-chan []byte
    metrics  chan<- Metric
}

func (w *Worker) Start() {
    for req := range w.reqChan {
        start := time.Now()
        _, err := w.conn.Write(req)
        if err != nil { ... }
        // read response
        end := time.Now()
        w.metrics <- Metric{Latency: end.Sub(start)}
    }
}

📦 7. Building

go build -o iso-stress-test main.go

🧪 8. Testing with a Local Server

To test TLS locally:

go run server.go -tls -cert=server.crt -key=server.key

Then run the stress tester.

📝 9. Notes

Ensure your server accepts persistent TCP connections.

Large ISO messages should be pre-built for efficiency.

Use WAL mode in SQLite if storing results.

For very high throughput (>10k RPS), increase worker count and tune OS TCP settings.

📜 10. License

MIT

🤝 Contributions

PRs welcome. Please open issues for enhancements or bugs.
