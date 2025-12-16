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
	"net"
	"os"
	"strings"
	"time"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/iso"
	"github.com/devarajang/longclaw/utils"
)

const ExpectedClientCN = "photon-client" // <--- Validate this

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

		isoMessage, err := iso.NewIso8583Message(string(message[0:n]), utils.GlobalIsoSpec)
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
	certPath string
	connMap  map[string]*IsoConnection
	db       *database.StressTestDB
}

func validateClient(connState tls.ConnectionState) error {

	if len(connState.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate presented")
	}

	clientCert := connState.PeerCertificates[0]

	// Check Org fields
	if clientCert.Subject.CommonName != ExpectedClientCN {

		return fmt.Errorf("invalid client Common name: %v",
			clientCert.Subject.CommonName)
	}

	log.Printf("Client certificate validated: Org=%v, CN=%s",
		clientCert.Subject.Organization,
		clientCert.Subject.CommonName)

	return nil
}

func (server *IsoServer) RunStress(stressTest database.StressTest, isoSpec *iso.IsoSpec) {
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

func sendSingleMessage(conn *IsoConnection, stressTest database.StressTest, isoSpec *iso.IsoSpec) {
	reference := utils.GenerateTimestampID()
	isoMessage, err := iso.NewIso8583Message(utils.RandomTemplate().Message, isoSpec)
	if err != nil {
		fmt.Println("ISO Error:", err)
		return
	}

	card := utils.GetRandomCard()

	fmt.Println(card)
	fmt.Println("Card number = ", isoMessage.GetField(1))
	//fmt.Println("Track1 = ", isoMessage.GetField(44))
	fmt.Println("Track2 = ", isoMessage.GetField(34))

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
}

func (server *IsoServer) GetConnectedClients() []string {
	keys := make([]string, len(server.connMap))

	for key, _ := range server.connMap {
		keys = append(keys, key)
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
	if err := tlsConn.Handshake(); err != nil {
		log.Println("TLS handshake error:", err)
		return
	}

	// Validate client certificate Org
	if err := validateClient(tlsConn.ConnectionState()); err != nil {
		log.Println("client cert validation failed:", err)
		return
	}

	isoConn := &IsoConnection{
		conn:         conn,
		readChannel:  make(chan IsoRequestResponse, 10),
		writeChannel: make(chan IsoRequestResponse, 10),
		closeChannel: make(chan struct{}),
		db:           server.db,
	}
	server.connMap[remoteAddr.String()] = isoConn

	go isoConn.HandleRead()
	go isoConn.HandleChannelEvents()
}

func (server *IsoServer) StartListen() error {

	// First create cert from certfile and keyfile
	cert, err := tls.LoadX509KeyPair(server.certPath+"server.crt", server.certPath+"server.key")
	if err != nil {
		log.Fatalf("failed to load server cert/key: %v", err)
		return err
	}

	caCert, err := os.ReadFile(server.certPath + "ca.crt")

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
		go server.HandleNewConnect(conn)
	}
}

func NewIsoServer(db *database.StressTestDB, certPath string) (*IsoServer, error) {

	server := IsoServer{
		certPath: certPath,
		connMap:  make(map[string]*IsoConnection),
		db:       db,
	}

	return &server, errors.New("Unable to create the server")
}
