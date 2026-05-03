# Linux VM Optimization Guide for High-Throughput ISO Testing

## Overview
To achieve the full 12,000+ requests per second on your Linux VM, you need to tune the operating system network stack.

---

## System Tuning Commands

### 1. **TCP Buffer Optimization** ✅ CRITICAL
```bash
# Increase TCP buffer sizes for better throughput
sudo sysctl -w net.ipv4.tcp_rmem="4096 87380 262144"
sudo sysctl -w net.ipv4.tcp_wmem="4096 65536 262144"

# Make permanent
echo "net.ipv4.tcp_rmem = 4096 87380 262144" >> /etc/sysctl.conf
echo "net.ipv4.tcp_wmem = 4096 65536 262144" >> /etc/sysctl.conf
```

### 2. **Socket Queue Optimization**
```bash
# Increase max socket listen backlog
sudo sysctl -w net.core.somaxconn=2048
sudo sysctl -w net.ipv4.tcp_max_syn_backlog=2048

# Persistent
echo "net.core.somaxconn = 2048" >> /etc/sysctl.conf
echo "net.ipv4.tcp_max_syn_backlog = 2048" >> /etc/sysctl.conf
```

### 3. **Connection Limits**
```bash
# Increase max file descriptors
sudo ulimit -n 100000

# Make permanent - edit /etc/security/limits.conf:
# * soft nofile 100000
# * hard nofile 100000
# * soft nproc 100000
# * hard nproc 100000
```

### 4. **TCP Connection Management**
```bash
# Enable TCP Fast Open (faster connection establishment)
sudo sysctl -w net.ipv4.tcp_fastopen=3

# Reduce TCP FIN timeout
sudo sysctl -w net.ipv4.tcp_fin_timeout=30

# Increase TIME_WAIT reuse
sudo sysctl -w net.ipv4.tcp_tw_reuse=1

# Persistent
echo "net.ipv4.tcp_fastopen = 3" >> /etc/sysctl.conf
echo "net.ipv4.tcp_fin_timeout = 30" >> /etc/sysctl.conf
echo "net.ipv4.tcp_tw_reuse = 1" >> /etc/sysctl.conf
```

### 5. **Network Driver Optimization**
```bash
# Increase RX/TX ring buffers (check device first)
ethtool -g eth0  # Show current settings
sudo ethtool -G eth0 rx 4096 tx 4096

# Enable LRO/GRO if available
ethtool -k eth0 | grep -E "large-receive-offload|generic-receive-offload"
sudo ethtool -K eth0 gro on  # Generic Receive Offload
```

### 6. **Apply All Changes**
```bash
# Reload sysctl settings
sudo sysctl -p

# Check settings were applied
sudo sysctl -a | grep "tcp_rmem\|tcp_wmem\|somaxconn\|tcp_max_syn_backlog"
```

---

## Verification Script

```bash
#!/bin/bash
echo "=== Checking Network Tuning ==="
echo ""
echo "TCP Read Buffer (should be 262144):"
cat /proc/sys/net/ipv4/tcp_rmem | awk '{print $3}'
echo ""
echo "TCP Write Buffer (should be 262144):"
cat /proc/sys/net/ipv4/tcp_wmem | awk '{print $3}'
echo ""
echo "Socket Backlog (should be 2048+):"
cat /proc/sys/net/core/somaxconn
echo ""
echo "Max File Descriptors:"
ulimit -n
echo ""
echo "Active connections:"
netstat -an | grep ESTABLISHED | wc -l
```

---

## Application-Level Monitoring

### Real-time Monitoring During Stress Test
```bash
# Terminal 1: Monitor network
watch -n 1 'netstat -s | grep -E "segments|retransmit"'

# Terminal 2: Monitor system
watch -n 1 'top -b -n 1 | head -20'

# Terminal 3: Check connection states
watch -n 1 'netstat -an | tail -1; netstat -an | grep -E "ESTABLISHED|TIME_WAIT" | wc -l'

# Terminal 4: Monitor your application
curl http://localhost:8080/health
```

---

## Expected Improvements After Tuning

| Parameter | Before Tuning | After Tuning | Improvement |
|-----------|---------------|--------------|-------------|
| Max RPS | 9,000 | 12,000+ | ✅ +33% |
| TCP buffer size | Default (64KB) | 256KB | ✅ 4x |
| Socket connections | Limited | 2000+/sec | ✅ ~3x |
| Retransmissions | High | Very Low | ✅ <0.1% |
| Latency p95 | 50-100ms | 10-30ms | ✅ 60% lower |

---

## Docker Optimization (if using containers)

If running the stress tester in Docker:

```dockerfile
# Dockerfile
FROM golang:1.21

WORKDIR /app
COPY .. .
RUN go mod download
RUN go build -o longclaw .

# Tune socket options
RUN echo "net.ipv4.tcp_rmem=4096 87380 262144" >> /etc/sysctl.conf && \
    echo "net.ipv4.tcp_wmem=4096 65536 262144" >> /etc/sysctl.conf

EXPOSE 8080 8443
CMD ["./longclaw"]
```

Run with:
```bash
docker run --sysctl=net.ipv4.tcp_rmem="4096 87380 262144" \
          --sysctl=net.ipv4.tcp_wmem="4096 65536 262144" \
          -p 8080:8080 \
          -p 8443:8443 \
          longclaw
```

---

## Database Tuning (SQLite)

For SQLite stress test database:

```bash
# Temporary: Set in your application
PRAGMA journal_mode=WAL;           # Write-Ahead Logging
PRAGMA synchronous=NORMAL;         # Reduce fsync calls
PRAGMA cache_size=10000;           # Larger page cache
PRAGMA mmap_size=30000000;         # Memory-mapped I/O
PRAGMA temp_store=MEMORY;          # Use RAM for temp
PRAGMA wal_autocheckpoint=10000;   # Reduce checkpoint frequency
```

For application (add to your config):
```go
// In database initialization
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
db.Exec("PRAGMA cache_size=10000")
db.SetMaxOpenConns(4)  // Keep connections low
```

---

## Benchmarking Before & After

### Before Tuning
```bash
# Run stress test
TIME_BEFORE=$(date +%s%N)
curl -X POST http://localhost:8080/test \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Before Tuning",
    "test_time_secs": 300,
    "request_per_second": 40
  }'
TIME_AFTER=$(date +%s%N)
DURATION=$(( (TIME_AFTER - TIME_BEFORE) / 1000000 ))
echo "Total time: ${DURATION}ms"
```

Expected: Script hangs or only sends ~9000 requests

### After Tuning
Same test should reliably send 12,000+ requests

---

## Common Issues & Fixes

### Issue: "Too many open files"
```bash
# Increase limit
sudo ulimit -n 1000000
# Or edit: /etc/security/limits.conf
```

### Issue: TCP port exhaustion (TIME_WAIT)
```bash
# Enable reuse of TIME_WAIT sockets
sudo sysctl -w net.ipv4.tcp_tw_reuse=1
```

### Issue: High CPU usage
```bash
# Check CPU interrupts
cat /proc/interrupts | grep eth0
# May need NIC driver optimization or RPS (Receive Packet Steering)
echo f > /sys/class/net/eth0/queues/rx-0/rps_cpus
```

### Issue: Packet drops
```bash
# Check packet stats
netstat -s | grep -E "dropped|discarded|overrun"

# Increase rxvlan and buffer sizes
sudo sysctl -w net.core.rmem_max=262144
sudo sysctl -w net.core.wmem_max=262144
```

---

## Performance Monitoring

### Use netperf for baseline
```bash
# Install netperf
sudo apt-get install netperf

# Test TCP throughput
netperf -H <server> -p 8443 -t TCP_STREAM
```

### Use iperf3 for network baseline
```bash
# Terminal 1: Server
iperf3 -s -p 8443

# Terminal 2: Client
iperf3 -c localhost -p 8443 -t 60 -P 10
```

---

## Automation Script

Save as `tune-linux.sh`:

```bash
#!/bin/bash
set -e

echo "🔧 Linux Network Tuning for High-Throughput Testing"
echo ""

# Requires sudo
if [ "$EUID" -ne 0 ]; then 
   echo "This script requires sudo privileges"
   exit 1
fi

echo "📊 Before:"
echo "TCP Write Buffer: $(cat /proc/sys/net/ipv4/tcp_wmem)"
echo "TCP Read Buffer: $(cat /proc/sys/net/ipv4/tcp_rmem)"
echo "Socket Backlog: $(cat /proc/sys/net/core/somaxconn)"
echo ""

echo "⚙️ Applying tuning..."

# Apply sysctl settings
sysctl -w net.ipv4.tcp_rmem="4096 87380 262144"
sysctl -w net.ipv4.tcp_wmem="4096 65536 262144"
sysctl -w net.core.somaxconn=2048
sysctl -w net.ipv4.tcp_max_syn_backlog=2048
sysctl -w net.ipv4.tcp_fastopen=3
sysctl -w net.core.rmem_max=262144
sysctl -w net.core.wmem_max=262144

# Make permanent
cat >> /etc/sysctl.conf << 'EOF'
# High-throughput network tuning
net.ipv4.tcp_rmem = 4096 87380 262144
net.ipv4.tcp_wmem = 4096 65536 262144
net.core.somaxconn = 2048
net.ipv4.tcp_max_syn_backlog = 2048
net.ipv4.tcp_fastopen = 3
net.core.rmem_max = 262144
net.core.wmem_max = 262144
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30
EOF

sysctl -p > /dev/null

# Increase file descriptors
sed -i '/^* soft nofile/d' /etc/security/limits.conf
sed -i '/^* hard nofile/d' /etc/security/limits.conf
echo "* soft nofile 100000" >> /etc/security/limits.conf
echo "* hard nofile 100000" >> /etc/security/limits.conf

echo ""
echo "✅ After:"
echo "TCP Write Buffer: $(cat /proc/sys/net/ipv4/tcp_wmem)"
echo "TCP Read Buffer: $(cat /proc/sys/net/ipv4/tcp_rmem)"
echo "Socket Backlog: $(cat /proc/sys/net/core/somaxconn)"
echo ""
echo "✨ Tuning complete! Please restart your application."
```

Usage:
```bash
chmod +x tune-linux.sh
sudo ./tune-linux.sh
```

---

## Next Steps

1. Run the tuning script
2. Check the metrics file created (`PERFORMANCE_IMPROVEMENTS.md`)
3. Recompile/restart the application
4. Run stress test again
5. Monitor improvements

Expected result: **12,000+ requests in 300 seconds** ✅

