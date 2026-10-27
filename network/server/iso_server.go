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
	"time"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/internal/config"
	"github.com/devarajang/longclaw/internal/domain"
	"github.com/devarajang/longclaw/iso"
	"github.com/devarajang/longclaw/utils"
)

var errTestInProgress = errors.New("connection test is already in progress")

type IsoRequestResponse struct {
	requestTime  time.Time
	responseTime time.Time
	rawRequest   []byte
	rawResponse  []byte
	reference    string
	mti          string // request MTI; with reference it identifies the response to wait for
	stressTestId int
}

// sentRequest is what sentTimes holds for a request awaiting its response
type sentRequest struct {
	mti    string
	sentAt time.Time
}

// pendingFollowUp is a reversal or advice to send once the original request's response arrives
type pendingFollowUp struct {
	mti          string
	rawMessage   string
	stressTestId int
}

// matchKey pairs a request with its response: same reference (field 37) and same MTI with the function
// digit made even, so 0100 and 0110 match but 0100 and 0420 carrying the same reference do not.
func matchKey(mti, reference string) string {
	return mti[:2] + string(mti[2]&^1) + mti[3:4] + "|" + reference
}

type IsoConnection struct {
	conn         net.Conn
	readChannel  chan IsoRequestResponse
	writeChannel chan IsoRequestResponse
	closeChannel chan struct{}
	db           *database.StressTestDB
	connTestMap  sync.Map
	closeOnce    sync.Once
	onClose      func()   // removes the connection from the server's map
	sentTimes    sync.Map // matchKey -> sentRequest, for requests written to the socket and not yet answered
	followUps    sync.Map // reference -> *pendingFollowUp, sent when the original's response arrives
}

// writeChannelSize is how many messages a connection can queue before new ones are dropped
const writeChannelSize = 1024

func (c *IsoConnection) HandleRead() {
	defer c.Close()
	for {
		message, err := readLengthPrefixedMessage(c.conn)
		// Stamp on read so time spent on our side afterwards (parsing, DB) is not counted as latency
		receivedAt := time.Now()
		if err != nil {
			log.Printf("Error: %v", err)
			return
		}
		n := len(message)
		// MTI + primary bitmap is the shortest frame that can be parsed
		if n < 20 {
			log.Printf("Dropping short message (%d bytes) from %v", n, c.conn.RemoteAddr())
			continue
		}
		msgBody := string(message[0:n])
		mti := msgBody[0:4]

		// Echo-test path: route 0810 responses back to our own 0800 pings.
		if (mti == "0810" || mti == "0800") && n >= 46 {
			stan := msgBody[36:46]
			if waitChan, ok := c.connTestMap.Load(stan); ok {
				waitChan.(chan string) <- msgBody
				continue
			}
		}

		isoMessage, err := iso.NewIso8583Message(msgBody, utils.GlobalIsoSpec)
		if err != nil {
			log.Printf("Parse error: %v", err)
			continue
		}

		switch {
		case isRequestMTI(mti):
			// buildResponse sleeps to simulate a slow issuer for some PANs; respond from a goroutine
			// so that delay does not hold up reading the next message on this connection
			go c.respond(isoMessage)
		case isResponseMTI(mti):
			reference := isoMessage.GetField(36)
			sent, ok := c.sentTimes.LoadAndDelete(matchKey(mti, reference))
			if !ok {
				unmatchedCount.Add(1)
				continue
			}
			request := sent.(sentRequest)
			// Only queues the write, so a slow database does not delay reading responses
			if err := c.db.UpdateResponseTime(reference, c.conn.RemoteAddr().String(), request.mti, request.sentAt, receivedAt,
				isoMessage.GetField(fieldIdxResponseCode), isoMessage.GetField(fieldIdxAuthID)); err != nil {
				log.Println("Response time update error:", err)
			}
			// The original is answered, so its reversal or advice can go now
			if followUp, ok := c.followUps.LoadAndDelete(reference); ok {
				c.sendFollowUp(reference, followUp.(*pendingFollowUp))
			}
		}
	}
}

// sendFollowUp logs and queues a reversal or advice whose original request has been answered
func (c *IsoConnection) sendFollowUp(reference string, followUp *pendingFollowUp) {
	connectionID := c.conn.RemoteAddr().String()
	now := time.Now()
	// Log before enqueueing so the row exists before the follow-up's response can arrive
	entry := domain.RequestLog{StressTestID: followUp.stressTestId, RequestTime: now, Reference: reference, ConnectionID: connectionID, MTI: followUp.mti}
	if err := c.db.AddRequestLog(entry); err != nil {
		log.Println("DB Write error:", err)
		return
	}
	select {
	case c.writeChannel <- IsoRequestResponse{reference: reference, mti: followUp.mti, stressTestId: followUp.stressTestId, rawRequest: []byte(followUp.rawMessage)}:
		if err := c.db.MarkScheduledMessageSent(reference, connectionID, now); err != nil {
			log.Println("DB Write error:", err)
		}
	default:
		droppedCount.Add(1)
		if err := c.db.DeleteRequestLog(reference, connectionID, followUp.mti); err != nil {
			log.Println("DB Write error:", err)
		}
	}
}

// respond builds the issuer response to an incoming request and queues it for writing
func (c *IsoConnection) respond(request *iso.Iso8583Message) {
	resp, err := buildResponse(request)
	if err != nil {
		log.Printf("Response build error: %v", err)
		return
	}
	select {
	case c.writeChannel <- IsoRequestResponse{rawRequest: []byte(resp.FormatIso())}:
	default:
		droppedCount.Add(1)
	}
}

func readLengthPrefixedMessage(conn net.Conn) ([]byte, error) {
	// Read 2-byte length prefix
	lengthBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, lengthBytes); err != nil {
		return nil, fmt.Errorf("failed to read length prefix: %v", err)
	}

	// Convert to length (big-endian)
	length := binary.BigEndian.Uint16(lengthBytes)

	// Read the actual message
	message := make([]byte, length)
	if _, err := io.ReadFull(conn, message); err != nil {
		return nil, fmt.Errorf("failed to read message: %v", err)
	}

	return message, nil
}

// Close shuts the connection down once; the read loop, write loop and TestConnection may all call it
func (sc *IsoConnection) Close() {
	sc.closeOnce.Do(func() {
		close(sc.closeChannel)
		sc.conn.Close()
		if sc.onClose != nil {
			sc.onClose()
		}
	})
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
		return false, errTestInProgress
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
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.conn.Write(buf)
	c.conn.SetWriteDeadline(time.Time{}) // Reset
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
	for {
		select {
		case msg := <-c.writeChannel:
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(len(msg.rawRequest)))
			buf = append(buf, msg.rawRequest...)

			if msg.reference != "" {
				// Stamp just before the write so latency excludes time queued on our side.
				// Stored before writing so a fast response can never arrive ahead of it.
				c.sentTimes.Store(matchKey(msg.mti, msg.reference), sentRequest{mti: msg.mti, sentAt: time.Now()})
			}
			_, err := c.conn.Write(buf)
			if err != nil {
				log.Println("Socket write error:", err)
				c.Close()
				return
			}
			sentCount.Add(1)
		case <-c.closeChannel:
			return
		}
	}
}

type IsoServer struct {
	listener net.Listener
	config   *config.Config
	connMap  map[string]*IsoConnection
	connMu   sync.RWMutex // guards connMap: written by connection goroutines, read by HTTP handlers and RunStress
	db       *database.StressTestDB
}

func (server *IsoServer) getConn(clientId string) (*IsoConnection, bool) {
	server.connMu.RLock()
	defer server.connMu.RUnlock()
	conn, ok := server.connMap[clientId]
	return conn, ok
}

// removeConn deletes clientId from connMap if it still maps to conn
func (server *IsoServer) removeConn(clientId string, conn *IsoConnection) {
	server.connMu.Lock()
	defer server.connMu.Unlock()
	if server.connMap[clientId] == conn {
		delete(server.connMap, clientId)
		log.Println("Client disconnected:", clientId)
	}
}

// connections returns a snapshot of the connected clients, safe to use without holding the lock.
// It is sorted by client ID so round_robin visits nodes in the same order on every tick.
func (server *IsoServer) connections() []*IsoConnection {
	server.connMu.RLock()
	defer server.connMu.RUnlock()
	ids := make([]string, 0, len(server.connMap))
	for id := range server.connMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	conns := make([]*IsoConnection, 0, len(ids))
	for _, id := range ids {
		conns = append(conns, server.connMap[id])
	}
	return conns
}

// nodeTargets returns the connections that get one step of a stress test. In advertised mode a step is
// one message to every node, so each node receives request_per_second. In round_robin mode a step is one
// message to the next node in turn, so request_per_second is the total across all nodes.
func nodeTargets(mode string, conns []*IsoConnection, next *int) []*IsoConnection {
	if mode != domain.NodeTestModeRoundRobin || len(conns) == 0 {
		return conns
	}
	i := *next % len(conns)
	*next++
	return conns[i : i+1]
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
	conn, ok := server.getConn(clientId)
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
	conn, ok := server.getConn(clientId)

	if ok == false {
		return "", errors.New("Client is not in the connected list")
	}
	testStatus, err := conn.TestConnection()

	if testStatus == true {
		log.Printf("Client socket validation:  %v,  %s", testStatus, err)
	} else if !errors.Is(err, errTestInProgress) {
		// Close also removes the client from connMap via onClose
		conn.Close()
		log.Println("Closed client after failed connection test:", clientId)
	}
	return fmt.Sprintf("Client socket validation:  %v,  %s", testStatus, err), nil
}

// stressWorkers is how many goroutines build and queue stress messages. It is bounded so a slow
// database or socket backs the generator up instead of piling up one goroutine per message.
const stressWorkers = 64

// RunStress sends RequestPerSecond messages per second to every connected client for TestTimeSecs.
// The number due is worked out from elapsed time, so late or dropped ticks are made up on the next one.
func (server *IsoServer) RunStress(stressTest domain.StressTest, isoSpec *iso.IsoSpec) {
	if stressTest.TestTimeSecs <= 0 || stressTest.RequestPerSecond <= 0 {
		log.Printf("Skipping stress test %d: test_time_secs and request_per_second must be greater than 0", stressTest.ID)
		return
	}
	// 1. Create a context with timeout based on stress test duration
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(stressTest.TestTimeSecs)*time.Second)
	defer cancel() // Ensure context is cancelled when function exits

	jobs := make(chan *IsoConnection, stressWorkers*16)
	var workers sync.WaitGroup
	for i := 0; i < stressWorkers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for conn := range jobs {
				sendSingleMessage(conn, stressTest, isoSpec)
			}
		}()
	}

	// Tick at the request interval, but no faster than 1ms: Go cannot deliver sub-millisecond ticks reliably
	interval := time.Second / time.Duration(stressTest.RequestPerSecond)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	steps := 0 // steps sent so far; see nodeTargets for what one step sends
	next := 0  // round_robin: position of the node that gets the next message
	count := 0 // messages queued in total
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case now := <-ticker.C:
			due := int(now.Sub(start).Seconds() * float64(stressTest.RequestPerSecond))
			conns := server.connections()
			for ; steps < due; steps++ {
				for _, conn := range nodeTargets(stressTest.NodeTestMode, conns, &next) {
					select {
					case jobs <- conn:
						count++
					case <-ctx.Done():
						break loop
					}
				}
			}
		}
	}
	close(jobs)
	workers.Wait()

	elapsed := time.Since(start)
	log.Printf("Stress test completed (%s): %d requests sent in %v (%.0f/s)", stressTest.NodeTestMode, count, elapsed.Round(time.Millisecond), float64(count)/elapsed.Seconds())
	if err := server.db.SetTotalRequests(stressTest.ID, count); err != nil {
		log.Println("DB Write error:", err)
	}
}

// newRequestLog is the request_response_log row for a message about to be sent. It keeps the
// request details a refund test needs to find and replay approved sales.
func newRequestLog(stressTestID int, reference, connectionID string, msg *iso.Iso8583Message) domain.RequestLog {
	return domain.RequestLog{
		StressTestID:   stressTestID,
		RequestTime:    time.Now(),
		Reference:      reference,
		ConnectionID:   connectionID,
		MTI:            msg.GetMTI(),
		ProcessingCode: msg.GetField(2),
		PAN:            msg.GetField(1),
		Amount:         msg.GetField(3),
		Currency:       msg.GetField(48),
		RequestMessage: msg.FormatIso(),
	}
}

func sendSingleMessage(conn *IsoConnection, stressTest domain.StressTest, isoSpec *iso.IsoSpec) {
	reference := utils.GenerateTimestampID()
	randomTemplate := utils.RandomTemplate()

	//var err error

	card := utils.GetRandomCard()

	getIsoMessage := func(message string) (*iso.Iso8583Message, error) {
		isoMessage, err := iso.NewIso8583Message(message, isoSpec)
		if err != nil {
			return nil, err
		}
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
				//de123[ind+2 : 2]
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
		return isoMessage, nil
	}

	var isoMessage, scheduledMessage *iso.Iso8583Message
	var err error
	if randomTemplate.OriginalMessage != "" {
		if isoMessage, err = getIsoMessage(randomTemplate.OriginalMessage); err == nil {
			scheduledMessage, err = getIsoMessage(randomTemplate.Message)
		}
	} else {
		isoMessage, err = getIsoMessage(randomTemplate.Message)
	}
	if err != nil {
		log.Println("Template parse error:", err)
		return
	}
	connectionID := conn.conn.RemoteAddr().String()
	if scheduledMessage != nil {
		// Recorded now with sent_at empty; HandleRead sends it once the original is answered.
		// Registered before the original is queued so its response cannot arrive first.
		conn.db.AddScheduledMessage(stressTest.ID, time.Now(), reference, connectionID, scheduledMessage.FormatIso())
		conn.followUps.Store(reference, &pendingFollowUp{
			mti:          scheduledMessage.GetMTI(),
			rawMessage:   scheduledMessage.FormatIso(),
			stressTestId: stressTest.ID,
		})
	}

	entry := newRequestLog(stressTest.ID, reference, connectionID, isoMessage)
	req := IsoRequestResponse{
		reference:    reference,
		mti:          entry.MTI,
		stressTestId: stressTest.ID,
		rawRequest:   []byte(entry.RequestMessage),
	}

	// Log before enqueueing so the row exists before any response can arrive.
	// request_time is replaced with the actual send time when the response is matched.
	if err := conn.db.AddRequestLog(entry); err != nil {
		log.Println("DB Write error:", err)
		conn.followUps.Delete(reference)
		return
	}

	select {
	case conn.writeChannel <- req:
	default:
		droppedCount.Add(1)
		// Never sent, so it must not count as a request without a response, and its follow-up never goes
		conn.followUps.Delete(reference)
		if err := conn.db.DeleteRequestLog(reference, connectionID, req.mti); err != nil {
			log.Println("DB Write error:", err)
		}
	}
}

func (server *IsoServer) GetConnectedClients() []string {
	server.connMu.RLock()
	defer server.connMu.RUnlock()
	keys := make([]string, 0, len(server.connMap))

	for key := range server.connMap {
		if len(key) > 0 {
			keys = append(keys, key)
		}
	}
	return keys
}

func (server *IsoServer) HandleNewConnect(conn net.Conn) {

	// Close the socket on every early return; once registered, IsoConnection.Close owns it
	registered := false
	defer func() {
		if !registered {
			conn.Close()
		}
	}()
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

	// Simulate random 8 percent disconnect after client cert is received.
	// Top-level rand functions are safe for concurrent use, unlike a shared *rand.Rand.
	roll := rand.Intn(100)
	log.Printf("[TLS] Random roll: %d (0-7=disconnect, 8-99=allow) for %v", roll, remoteAddr)
	if roll < 8 {
		log.Printf("[TLS] Simulated disconnect after handshake from %v", remoteAddr)
		return
	}

	// Validate client certificate Org
	if err := validateClient(tlsConn.ConnectionState(), server.config.TLS.ExpectedCN); err != nil {
		log.Println("client cert validation failed:", err)
		return
	}

	isoConn := &IsoConnection{
		conn:         conn,
		readChannel:  make(chan IsoRequestResponse, 16),
		writeChannel: make(chan IsoRequestResponse, writeChannelSize),
		closeChannel: make(chan struct{}),
		db:           server.db,
	}
	clientId := remoteAddr.String()
	isoConn.onClose = func() { server.removeConn(clientId, isoConn) }
	server.connMu.Lock()
	server.connMap[clientId] = isoConn
	server.connMu.Unlock()
	registered = true

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

	listener, err := tls.Listen("tcp", ":8443", tlsConfig)

	if err != nil {
		return err
	}
	server.connMu.Lock()
	server.listener = listener
	server.connMu.Unlock()
	log.Println("Started TLS socket server", listener.Addr())
	go reportSentPerMinute()
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Stop closed the listener: a normal shutdown, not a failure
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		log.Printf("[TCP] New connection from %v", conn.RemoteAddr())
		go server.HandleNewConnect(conn)
	}
}

// Stop closes the listener, so StartListen returns nil, and then every client connection
func (server *IsoServer) Stop() {
	server.connMu.RLock()
	listener := server.listener
	server.connMu.RUnlock()
	if listener != nil {
		listener.Close()
	}
	for _, conn := range server.connections() {
		conn.Close()
	}
}

func NewIsoServer(db *database.StressTestDB, cfg *config.Config) (*IsoServer, error) {

	server := IsoServer{
		config:  cfg,
		connMap: make(map[string]*IsoConnection),
		db:      db,
	}

	return &server, errors.New("Unable to create the server")
}
