package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/internal/config"
	"github.com/devarajang/longclaw/internal/domain"
	"github.com/devarajang/longclaw/iso"
	"github.com/devarajang/longclaw/utils"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

type IsoRequestResponse struct {
	requestTime  time.Time
	responseTime time.Time
	rawRequest   []byte
	rawResponse  []byte
	reference    string
	stressTestId int
}

type IsoConnection struct {
	conn            net.Conn
	readChannel     chan IsoRequestResponse
	writeChannel    chan IsoRequestResponse
	closeChannel    chan struct{}
	db              *database.StressTestDB
	connTestMap     sync.Map
	remoteAddr      string
	server          *IsoServer
	writeSuccess    atomic.Int64
	writeFailed     atomic.Int64
	writeErrorsMu   sync.Mutex
	writeErrorTypes map[string]int64 // error classification -> count
	receivedCount   atomic.Int64     // count of messages received (for batched logging)
}

func (c *IsoConnection) recordWriteError(err error) {
	var key string
	var netErr net.Error
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		key = "timeout"
	default:
		msg := err.Error()
		switch {
		case containsAny(msg, "broken pipe", "connection reset", "forcibly closed"):
			key = "broken_pipe/reset"
		case containsAny(msg, "use of closed", "closed network"):
			key = "use_of_closed"
		case containsAny(msg, "EOF"):
			key = "eof"
		default:
			key = "other: " + msg
		}
	}
	c.writeErrorsMu.Lock()
	if c.writeErrorTypes == nil {
		c.writeErrorTypes = make(map[string]int64)
	}
	c.writeErrorTypes[key]++
	c.writeErrorsMu.Unlock()
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func (c *IsoConnection) resetWriteStats() {
	c.writeSuccess.Store(0)
	c.writeFailed.Store(0)
	c.receivedCount.Store(0)
	c.writeErrorsMu.Lock()
	c.writeErrorTypes = make(map[string]int64)
	c.writeErrorsMu.Unlock()
}

func (c *IsoConnection) writeErrorSummary() map[string]int64 {
	c.writeErrorsMu.Lock()
	defer c.writeErrorsMu.Unlock()
	out := make(map[string]int64, len(c.writeErrorTypes))
	for k, v := range c.writeErrorTypes {
		out[k] = v
	}
	return out
}

func (c *IsoConnection) HandleRead() {
	defer func() {
		close(c.closeChannel)
		_ = c.conn.Close()
		if c.server != nil {
			delete(c.server.connMap, c.remoteAddr)
			log.Printf("[CLEANUP] Connection removed from map: %s", c.remoteAddr)
		}
		// Log final received count
		finalCount := c.receivedCount.Load()
		if finalCount > 0 {
			log.Printf("[RECEIVED] Final count from %s: %d messages", c.remoteAddr, finalCount)
		}
	}()

	// Start a ticker to log received count every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			count := c.receivedCount.Load()
			if count > 0 {
				log.Printf("[RECEIVED] %s: %d messages in last 60 seconds", c.remoteAddr, count)
				c.receivedCount.Store(0) // Reset counter
			}
		}
	}()

	for {
		// Use longer timeout for idle clients (stress test may have idle periods)
		_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		message, err := readLengthPrefixedMessage(c.conn)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Timeout is non-fatal, keep connection open
				continue
			}
			// Check for wrapped timeout
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			log.Printf("[READ] Closed from %s: %v", c.remoteAddr, err)
			return
		}
		n := len(message)
		c.receivedCount.Add(1) // Increment counter instead of logging each message

		msgBody := string(message[0:n])
		if len(msgBody) > 46 && (msgBody[0:4] == "0810" || msgBody[0:4] == "0800") {
			stan := msgBody[36:46]
			if waitChan, ok := c.connTestMap.Load(stan); ok {
				select {
				case waitChan.(chan string) <- msgBody:
				default:
				}
			}
		}

		go func(msg string) {
			isoMessage, err := iso.NewIso8583Message(msg, utils.GlobalIsoSpec)
			if err == nil {
				ref := isoMessage.GetField(36)
				_ = c.db.UpdateResponseTime(ref, c.remoteAddr)
			}
		}(msgBody)
	}
}
func readLengthPrefixedMessage(conn net.Conn) ([]byte, error) {
	// Read 2-byte length prefix
	lengthBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, lengthBytes); err != nil {
		return nil, fmt.Errorf("failed to read length prefix: %w", err)
	}

	// Convert to length (big-endian)
	length := binary.BigEndian.Uint16(lengthBytes)

	// Read the actual message
	message := make([]byte, length)
	if _, err := io.ReadFull(conn, message); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	return message, nil
}

func (sc *IsoConnection) Close() {
	close(sc.closeChannel)
	_ = sc.conn.Close()
}

func (c *IsoConnection) TestConnection() (bool, error) {
	if c.conn == nil {
		return false, errors.New("connection is nil")
	}
	isEmpty := true
	c.connTestMap.Range(func(key, value any) bool {
		isEmpty = false
		return isEmpty
	})
	if !isEmpty {
		return false, errors.New("connection test is already in progress")
	}
	formatString := time.Now().Format("0102150405")
	isoMessage := "080082200000000000000400000000000000" + formatString + "026612361"

	waitChan := make(chan string, 1)
	c.connTestMap.Store(formatString, waitChan)
	defer c.connTestMap.Delete(formatString)
	// Prefix with 2-byte length (BigEndian)
	msgLen := len(isoMessage)
	buf := make([]byte, 2+msgLen)
	binary.BigEndian.PutUint16(buf[:2], uint16(msgLen))
	copy(buf[2:], isoMessage)

	// Set a timeout for the write operation
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.conn.Write(buf)
	_ = c.conn.SetWriteDeadline(time.Time{}) // Reset
	if err != nil {
		return false, err
	}
	select {
	case str := <-waitChan:
		//parsed, _ := iso.NewIso8583Message(str,)
		return str[0:4] == "0810" || str[0:4] == "0800", nil
	case <-time.After(5 * time.Second):
		return false, errors.New("timeout: no 0810 received for STAN " + formatString)
	}
}

func (c *IsoConnection) HandleChannelEvents() {
	// NOTE: We intentionally do NOT set a write deadline here.
	//
	// Why: Go's crypto/tls requires writes to be atomic. If SetWriteDeadline fires
	// mid-write, the TLS record layer is left in a corrupt state, and every
	// subsequent Write() on that connection fails immediately — even though the
	// underlying TCP session is still alive. This caused the "1000+ consecutive
	// write errors" cascade observed during stress tests.
	//
	// Instead, we rely on channel-level backpressure:
	//   - writeChannel has a fixed capacity (512 slots).
	//   - sendSingleMessage uses a non-blocking send, dropping and counting
	//     messages as "queue-dropped" when the channel is full.
	//   - If TCP send buffer is full, this goroutine blocks on Write() — that is
	//     intentional, as it is the dedicated writer goroutine for this connection.
	//   - TCP keepalive (set on the raw conn below) will detect dead peers.
	//
	// A true dead-peer scenario (where Write blocks forever) is caught by
	// HandleRead: when the peer closes the connection, HandleRead returns, closes
	// closeChannel, and this select exits via the closeChannel case.

	const maxLoggedConsecutiveWriteErrors = 3
	const suppressedLogInterval = 10 * time.Second
	consecutiveWriteErrors := 0
	suppressWriteErrorLogs := false
	suppressedErrorCount := 0
	lastSuppressedLogAt := time.Time{}

	for {
		select {
		case msg := <-c.writeChannel:
			if msg.reference == "" {
				continue
			}

			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(len(msg.rawRequest)))
			buf = append(buf, msg.rawRequest...)

			// No write deadline — see comment above.
			_, err := c.conn.Write(buf)

			if err != nil {
				c.writeFailed.Add(1)
				c.recordWriteError(err)
				consecutiveWriteErrors++
				if consecutiveWriteErrors <= maxLoggedConsecutiveWriteErrors {
					log.Printf("[WRITE] Error to %s (consecutive=%d): %v", c.remoteAddr, consecutiveWriteErrors, err)
				} else if !suppressWriteErrorLogs {
					suppressWriteErrorLogs = true
					suppressedErrorCount = 1
					lastSuppressedLogAt = time.Now()
					log.Printf("[WRITE] Too many consecutive errors to %s (> %d). Suppressing further write error logs until recovery.", c.remoteAddr, maxLoggedConsecutiveWriteErrors)
				} else {
					suppressedErrorCount++
					now := time.Now()
					if now.Sub(lastSuppressedLogAt) >= suppressedLogInterval {
						log.Printf("[WRITE] Still suppressed for %s: %d additional write errors in last interval (consecutive=%d)", c.remoteAddr, suppressedErrorCount, consecutiveWriteErrors)
						suppressedErrorCount = 0
						lastSuppressedLogAt = now
					}
				}
				// Keep connection open — this is a stress test tool.
				// Drop the failed message and continue draining the write channel.
				continue
			}

			if consecutiveWriteErrors > 0 {
				if suppressWriteErrorLogs && suppressedErrorCount > 0 {
					log.Printf("[WRITE] Recovered for %s after %d consecutive write errors (%d errors were suppressed). Resuming normal error logging.", c.remoteAddr, consecutiveWriteErrors, suppressedErrorCount)
				} else {
					log.Printf("[WRITE] Recovered for %s after %d consecutive write errors. Resuming normal error logging.", c.remoteAddr, consecutiveWriteErrors)
				}
				consecutiveWriteErrors = 0
				suppressWriteErrorLogs = false
				suppressedErrorCount = 0
				lastSuppressedLogAt = time.Time{}
			}
			c.writeSuccess.Add(1)

		case <-c.closeChannel:
			return
		}
	}
}

type IsoServer struct {
	listener net.Listener
	config   *config.Config
	connMap  map[string]*IsoConnection
	db       *database.StressTestDB
}

func validateClient(connState tls.ConnectionState, expectedCN string) error {

	if len(connState.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate presented")
	}

	clientCert := connState.PeerCertificates[0]

	// Check Org fields
	if clientCert.Subject.CommonName != expectedCN {

		return fmt.Errorf("invalid client Common name: %v",
			clientCert.Subject.CommonName)
	}

	log.Printf("Client certificate validated: Org=%v, CN=%s",
		clientCert.Subject.Organization,
		clientCert.Subject.CommonName)

	return nil
}

func (server *IsoServer) SendMessage(clientId string, rawMsg string) error {
	conn, ok := server.connMap[clientId]
	if !ok {
		return errors.New("client not found: " + clientId)
	}
	req := IsoRequestResponse{
		rawRequest: []byte(rawMsg),
	}
	select {
	case conn.writeChannel <- req:
		return nil
	default:
		return errors.New("write channel full for client: " + clientId)
	}
}

func (server *IsoServer) TestConnection(clientId string) (string, error) {
	conn, ok := server.connMap[clientId]

	if ok == false {
		return "", errors.New("Client is not in the connected list")
	}
	testStatus, err := conn.TestConnection()

	if testStatus == true {
		log.Printf("Client socket validation:  %v,  %s", testStatus, err)
	} else {
		delete(server.connMap, clientId)
		log.Println(server.connMap)
	}
	return fmt.Sprintf("Client socket validation:  %v,  %s", testStatus, err), nil
}

func (server *IsoServer) RunStress(stressTest domain.StressTest, isoSpec *iso.IsoSpec) {
	// 1. Create a context with timeout based on stress test duration
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(stressTest.TestTimeSecs)*time.Second)
	defer cancel()

	// Calculate interval for desired RPS
	interval := time.Second / time.Duration(stressTest.RequestPerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	count := 0                // Track number of ticks
	totalMessagesSent := 0    // Track actual messages sent
	totalMessagesDropped := 0 // Track dropped messages
	perClientSent := make(map[string]int)
	perClientDropped := make(map[string]int)

	resetMetrics := func() {
		count = 0
		totalMessagesSent = 0
		totalMessagesDropped = 0
		clear(perClientSent)
		clear(perClientDropped)
	}
	defer func() {
		resetMetrics()
		log.Printf("[STRESS] Metrics reset for test id=%d", stressTest.ID)
	}()

	// Pre-capture initial clients and log
	connectedClients := server.GetConnectedClients()
	initialClientCount := len(connectedClients)
	log.Printf("[STRESS] Starting test with %d clients: %v", initialClientCount, connectedClients)
	for _, c := range connectedClients {
		perClientSent[c] = 0
		perClientDropped[c] = 0
		// Reset per-connection socket write counters so each test run starts clean.
		if isoConn, ok := server.connMap[c]; ok {
			isoConn.resetWriteStats()
		}
	}

	// Pre-calculate total expected messages: one selected client per tick
	totalMessagesExpected := stressTest.RequestPerSecond * stressTest.TestTimeSecs
	roundRobinIndex := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Stress test completed: %d ticks, %d total messages sent, %d dropped (expected: %d)\n", count, totalMessagesSent, totalMessagesDropped, totalMessagesExpected)
			fmt.Printf("[SUMMARY] Per-client stats:\n")
			for _, clientAddr := range connectedClients {
				sent := perClientSent[clientAddr]
				dropped := perClientDropped[clientAddr]
				total := sent + dropped
				successRate := 0.0
				if total > 0 {
					successRate = float64(sent) / float64(total) * 100
				}
				conn, stillConnected := server.connMap[clientAddr]
				status := "ACTIVE"
				if !stillConnected {
					status = "DISCONNECTED"
				}
				var wSuccess, wFailed int64
				var errTypes map[string]int64
				if stillConnected && conn != nil {
					wSuccess = conn.writeSuccess.Load()
					wFailed = conn.writeFailed.Load()
					errTypes = conn.writeErrorSummary()
				}
				fmt.Printf("  %s: queued-sent=%d, queued-dropped=%d, queue-success=%.1f%% | socket-written=%d, socket-failed=%d [%s]\n",
					clientAddr, sent, dropped, successRate, wSuccess, wFailed, status)
				if len(errTypes) > 0 {
					fmt.Printf("    Write error breakdown:\n")
					for errType, errCount := range errTypes {
						fmt.Printf("      %-30s : %d\n", errType, errCount)
					}
				}
			}
			fmt.Printf("\n[DATABASE] Query logs in database:\n")
			fmt.Printf("  SELECT COUNT(*) FROM request_response_log WHERE stresstest_id=%d\n", stressTest.ID)
			fmt.Printf("  SELECT connection_id, COUNT(*) FROM request_response_log WHERE stresstest_id=%d GROUP BY connection_id\n", stressTest.ID)
			fmt.Printf("\n[NOTE] Database logs may be lower than messages sent due to async write failures.\n")
			fmt.Printf("       Check logs for [DB_ERROR] to see write failures.\n")
			return
		case <-ticker.C:
			// Send to one currently connected client per tick using round-robin.
			liveClients := server.GetConnectedClients()
			if len(liveClients) == 0 {
				totalMessagesDropped++
				count++
				continue
			}

			// Keep order stable so each client gets an even share over time.
			sort.Strings(liveClients)
			if roundRobinIndex >= len(liveClients) {
				roundRobinIndex = 0
			}
			connName := liveClients[roundRobinIndex]
			roundRobinIndex++
			conn, ok := server.connMap[connName]
			if !ok {
				perClientDropped[connName]++
				totalMessagesDropped++
				count++
				continue
			}

			if _, seen := perClientSent[connName]; !seen {
				perClientSent[connName] = 0
				perClientDropped[connName] = 0
			}

			// Use non-blocking send to avoid blocking
			if sendSingleMessage(conn, stressTest, isoSpec) {
				totalMessagesSent++
				perClientSent[connName]++
			} else {
				totalMessagesDropped++
				perClientDropped[connName]++
			}
			count++
		}
	}
}

func sendSingleMessage(conn *IsoConnection, stressTest domain.StressTest, isoSpec *iso.IsoSpec) bool {
	reference := utils.GenerateTimestampID()
	randomTemplate := utils.RandomTemplate()

	card := utils.GetRandomCard()

	getIsoMessage := func(message string) (*iso.Iso8583Message, error) {
		isoMessage, err := iso.NewIso8583Message(message, isoSpec)
		if isoMessage.GetField(1) != "" {
			isoMessage.SetField(1, card.CardNumber)
		}
		if isoMessage.GetField(13) != "" {
			isoMessage.SetField(13, card.ExpiryDate)
		}
		if isoMessage.GetField(34) != "" {
			isoMessage.SetField(34, card.Track2Data)
		}
		if isoMessage.GetField(122) != "" {
			de123 := isoMessage.GetField(122)
			ind := strings.Index(de123, "CV")

			if ind > -1 {
				lenStr := utils.Substr(de123, ind+2, 2)
				sb := strings.Builder{}
				sb.WriteString(utils.Substr(de123, 0, ind))

				switch lenStr {
				case "05":
					sb.WriteString("CV051 ")
					sb.WriteString(card.CVV2)
				case "07":
					sb.WriteString("CV0711 ")
					sb.WriteString(card.CVV2)
					sb.WriteString("M")
				}
				isoMessage.SetField(122, sb.String())
			}
		}

		isoMessage.SetField(36, reference)
		return isoMessage, err
	}

	var isoMessage, scheduledMessage *iso.Iso8583Message
	requestTime := time.Now()

	if randomTemplate.OriginalMessage != "" {
		isoMessage, _ = getIsoMessage(randomTemplate.OriginalMessage)
		scheduledMessage, _ = getIsoMessage(randomTemplate.Message)

		// Queue async DB write for scheduled message (fires immediately, writes happen in background)
		conn.db.AddScheduledMessage(stressTest.ID, requestTime, reference, conn.conn.RemoteAddr().String(), scheduledMessage.FormatIso())
	} else {
		isoMessage, _ = getIsoMessage(randomTemplate.Message)
	}

	req := IsoRequestResponse{
		reference:    reference,
		stressTestId: stressTest.ID,
		rawRequest:   []byte(isoMessage.FormatIso()),
	}

	// Non-blocking send
	select {
	case conn.writeChannel <- req:
		// Queue async DB write for request log (fires immediately, writes happen in background)
		conn.db.AddRequestLog(stressTest.ID, requestTime, reference, conn.conn.RemoteAddr().String())
		return true
	default:
		log.Printf("[DROP] Write channel full for %s (buffer: %d/%d)", conn.remoteAddr, len(conn.writeChannel), cap(conn.writeChannel))
		return false
	}
}

func (server *IsoServer) GetConnectedClients() []string {
	keys := make([]string, 0, len(server.connMap))

	for key := range server.connMap {
		if len(key) > 0 {
			keys = append(keys, key)
		}
	}
	return keys
}

func (server *IsoServer) HandleNewConnect(conn net.Conn) {

	//defer conn.Close()
	remoteAddr := conn.RemoteAddr()

	tlsConn, ok := conn.(*tls.Conn)

	if !ok {
		log.Printf("not a TLS connection from %v", remoteAddr)
		return
	}
	// Complete the handshake so certs are available
	log.Printf("[TLS] Starting handshake with %v", remoteAddr)
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("[TLS] Handshake failed from %v: %v", remoteAddr, err)
		return
	}
	log.Printf("[TLS] Handshake complete with %v", remoteAddr)

	// Enable TCP keepalive on the underlying connection so the OS detects
	// silently dead peers (no FIN/RST) and unblocks any pending Write().
	if tcpConn, ok := tlsConn.NetConn().(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	// Simulate random 8 percent disconnect after client cert is received
	/*roll := rng.Intn(100)
	log.Printf("[TLS] Random roll: %d (0=disconnect, 1=allow) for %v", roll, remoteAddr)
	if roll <= 8 {
		log.Printf("[TLS] Simulated disconnect after handshake from %v", remoteAddr)
		conn.Close()
		return
	}*/

	// Validate client certificate Org
	if err := validateClient(tlsConn.ConnectionState(), server.config.TLS.ExpectedCN); err != nil {
		log.Println("client cert validation failed:", err)
		return
	}

	isoConn := &IsoConnection{
		conn:         conn,
		readChannel:  make(chan IsoRequestResponse, 256),
		writeChannel: make(chan IsoRequestResponse, 512),
		//readChannel:  make(chan IsoRequestResponse, 512),
		//writeChannel: make(chan IsoRequestResponse, 4096),
		closeChannel: make(chan struct{}),
		db:           server.db,
		remoteAddr:   remoteAddr.String(),
		server:       server,
	}
	server.connMap[remoteAddr.String()] = isoConn

	go isoConn.HandleRead()
	go isoConn.HandleChannelEvents()
}

func (server *IsoServer) StartListen() error {

	// First create cert from certfile and keyfile
	cert, err := tls.LoadX509KeyPair(server.config.TLS.ServerCertPath, server.config.TLS.ServerKeyPath)
	if err != nil {
		log.Fatalf("failed to load server cert/key: %v", err)
		return err
	}

	caCert, err := os.ReadFile(server.config.TLS.CAPath)

	if err != nil {
		log.Fatalf("failed to load ca cert: %v", err)
		return err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		// ClientAuth: tls.RequestClientCert,
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert, // <--- require mTLS
		MinVersion: tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", server.config.Server.ISOPort, tlsConfig)

	if err != nil {
		return err
	}
	server.listener = listener
	log.Println("Started TLS socket server", listener.Addr())
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			return err
		}
		log.Printf("[TCP] New connection from %v", conn.RemoteAddr())
		go server.HandleNewConnect(conn)
	}
}

func NewIsoServer(db *database.StressTestDB, cfg *config.Config) (*IsoServer, error) {

	server := IsoServer{
		config:  cfg,
		connMap: make(map[string]*IsoConnection),
		db:      db,
	}

	return &server, nil
}
