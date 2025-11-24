🚀 ISO Socket Stress Test Tool
A high-performance stress test tool designed to generate ISO8583-style socket traffic against a server.
The tool supports:
🔌 TCP / TLS (SSL) socket communication
🧵 Parallel workers (goroutine-based)
📈 User-configurable Requests Per Second (RPS)
🔐 Optional client certificate authentication
✍️ Logging request/response and round-trip latency
🧱 Safe concurrency with channels

📑 1. Overview
This stress test tool sends ISO-formatted requests to a server at a controlled rate and measures:
Throughput (req/sec)
Success/failure counts
Network latency (per request)
Connection health
Server saturation behavior
It is useful for:
Load testing issuer/acquirer simulators
Benchmarking ISO8583 switch performance
Validating socket-based fintech systems
TLS handshake and connection-reuse testing

┌───────────────────────────┐
│        Test Runner        │
│   - RPS scheduler         │
│   - Worker manager        │
└─────────────┬─────────────┘
              │ goroutines
┌─────────────▼─────────────┐
│       Worker Pool         │
│ - socket connect          │
│ - send/receive            │
│ - record metrics          │
└─────────────┬─────────────┘
              │ channels
┌─────────────▼─────────────┐
│ Metrics Aggregator        │
│ - latency distribution    │
│ - success/fail counters   │
│ - QPS calculation         │
└───────────────────────────┘


Key concepts:
Each worker has its own TCP/TLS socket
Channel used for request scheduling
RPS controller controls the rate
Metrics aggregated in a safe single writer goroutine

🔐 4. TLS / SSL Setup
Generate CA, server, and client certificates
Server Key + Certificate:

Server Key + Certificate:

openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr
openssl x509 -req -in server.csr -signkey server.key -out server.crt -days 365


Client Key + Cert + PFX (optional_cert_auth):
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr
openssl x509 -req -in client.csr -signkey client.key -out client.crt -days 365

# Convert to .pfx
openssl pkcs12 -export -out client.pfx -inkey client.key -in client.crt -certfile server.crt

