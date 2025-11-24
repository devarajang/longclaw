package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func main() {

	certPath := "/Users/deva/workspace/goworkspace/longclaw/certs/"
	fmt.Println("Running a TLS client", certPath)

	caBytes, err := os.ReadFile(certPath + "server/ca.crt")

	if err != nil {
		panic(err.Error())
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBytes)

	// Load from client cert / key
	clientCert, err := tls.LoadX509KeyPair(certPath+"client/client.crt", certPath+"client/client.key")

	if err != nil {
		log.Fatalf("load client cert: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", "127.0.0.1:8443", tlsConfig)
	if err != nil {
		log.Fatalf("connect err: %v", err)
	}

	for {

		// Usage
		log.Println("Reading length-prefixed message from", conn.RemoteAddr())
		message, err := readLengthPrefixedMessage(conn)
		n := len(message)
		if err != nil {
			log.Printf("Error: %v", err)
			return
		}
		log.Printf("Received %d bytes: %s", n, string(message))

		time.Sleep(time.Millisecond * time.Duration(3000))
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(n))
		buf = append(buf, message[0:n]...)
		log.Println("Writing data to ", conn.RemoteAddr())
		conn.Write(buf)
		log.Println("Wrote data ", string(message[0:n]))
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
