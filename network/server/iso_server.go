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
	"strings"
	"sync"
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
	conn         net.Conn
	readChannel  chan IsoRequestResponse
	writeChannel chan IsoRequestResponse
	closeChannel chan struct{}
	db           *database.StressTestDB
	connTestMap  sync.Map
}

func (c *IsoConnection) HandleRead() {

	for {
		// buf := make([]byte, 4096)
		// n, err := c.conn.Read(buf)
		// Usage
		//log.Println("Reading length-prefixed message from", c.conn.RemoteAddr())
		message, err := readLengthPrefixedMessage(c.conn)
		if err != nil {
			log.Printf("Error: %v", err)
			return
		}
		n := len(message)
		log.Printf("Received %d bytes: %s", n, string(message))

		req := IsoRequestResponse{
			//requestTime:  time.Now(),
			responseTime: time.Now(),
			rawRequest:   message[0:n],
		}
		//log.Println("Reading data from ", c.conn.RemoteAddr())
		//log.Println("Read data:", string(req.rawRequest))
		msgBody := string(message[0:n])
		if msgBody[0:4] == "0810" || msgBody[0:4] == "0800" {
			stan := msgBody[36:46]
			if waitChan, ok := c.connTestMap.Load(stan); ok {
				waitChan.(chan string) <- msgBody
			}
		}

		isoMessage, err := iso.NewIso8583Message(msgBody, utils.GlobalIsoSpec)
		req.reference = isoMessage.GetField(36)

		c.db.UpdateResponseTime(req.reference, c.conn.RemoteAddr().String())
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

func (sc *IsoConnection) Close() {
	close(sc.closeChannel)
	sc.conn.Close()
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

			log.Println("Writing data to ", msg.reference, c.conn.RemoteAddr())

			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(len(msg.rawRequest)))
			buf = append(buf, msg.rawRequest...)

			_, err := c.conn.Write(buf)
			// Write to  database once the write to socket is done
			//var t time.Time
			err = c.db.AddRequestLog(msg.stressTestId, time.Now(),
				msg.reference, c.conn.RemoteAddr().String())
			if err != nil {
				log.Println("DB Write error:", err)
				return
			}
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
	defer cancel() // Ensure context is cancelled when function exits

	// Calculate interval for desired RPS
	interval := time.Second / time.Duration(stressTest.RequestPerSecond)
	ticker := time.NewTicker(interval) // Creates a ticker that fires at the calculated interval
	defer ticker.Stop()

	count := 0 // Track number of iterations
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Stress test completed: %d requests sent\n", count)
			return
		case <-ticker.C:
			// Send to all connections
			for _, connName := range server.GetConnectedClients() {
				conn, ok := server.connMap[connName]
				if !ok {
					continue
				}

				go sendSingleMessage(conn, stressTest, isoSpec)
			}
			count++
		}
	}
}

func sendSingleMessage(conn *IsoConnection, stressTest domain.StressTest, isoSpec *iso.IsoSpec) {
	reference := utils.GenerateTimestampID()
	randomTemplate := utils.RandomTemplate()

	//var err error

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
				fmt.Println(de123)
				fmt.Println(sb.String())
				isoMessage.SetField(122, sb.String())
			}
		}

		isoMessage.SetField(36, reference)
		return isoMessage, err
	}

	var isoMessage, scheduledMessage *iso.Iso8583Message
	if randomTemplate.OriginalMessage != "" {
		isoMessage, _ = getIsoMessage(randomTemplate.OriginalMessage)
		scheduledMessage, _ = getIsoMessage(randomTemplate.Message)
		conn.db.AddScheduledMessage(stressTest.ID, time.Now(), reference, conn.conn.RemoteAddr().String(), scheduledMessage.FormatIso())
	} else {
		isoMessage, _ = getIsoMessage(randomTemplate.Message)
	}

	/*if err != nil {
	fmt.Println("ISO Error:", err)
	return
	}*/
	req := IsoRequestResponse{
		reference:    reference,
		stressTestId: stressTest.ID,
		rawRequest:   []byte(isoMessage.FormatIso()),
	}

	select {
	case conn.writeChannel <- req:
	default:
		fmt.Println("Write channel full, skipping message")
	}
	if randomTemplate.OriginalMessage != "" {
		req = IsoRequestResponse{
			reference:    utils.GenerateTimestampID(),
			stressTestId: stressTest.ID,
			rawRequest:   []byte(scheduledMessage.FormatIso()),
		}
		select {
		case conn.writeChannel <- req:
		default:
			fmt.Println("Write channel full, skipping message")
		}
	}
}

func (server *IsoServer) GetConnectedClients() []string {
	keys := make([]string, len(server.connMap))

	for key, _ := range server.connMap {
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

	// Simulate random 8 percent disconnect after client cert is received
	roll := rng.Intn(100)
	log.Printf("[TLS] Random roll: %d (0=disconnect, 1=allow) for %v", roll, remoteAddr)
	if roll <= 8 {
		log.Printf("[TLS] Simulated disconnect after handshake from %v", remoteAddr)
		conn.Close()
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
		writeChannel: make(chan IsoRequestResponse, 16),
		closeChannel: make(chan struct{}),
		db:           server.db,
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

	listener, err := tls.Listen("tcp", ":8443", tlsConfig)

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

	return &server, errors.New("Unable to create the server")
}
