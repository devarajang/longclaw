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
	"strconv"
	"time"
)

// responseMTI returns the response MTI for a request or advice message (0100 -> 0110, 0420 -> 0430).
// The third MTI digit is the message function: even for requests/advices, odd for their responses.
func responseMTI(message string) (string, bool) {
	if len(message) < 4 || message[2] < '0' || message[2] > '9' || (message[2]-'0')%2 != 0 {
		return "", false
	}
	return message[:2] + string(message[2]+1) + message[3:4], true
}

func main() {

	certPath := getEnv("LONGCLAW_CERT_PATH", "certs/")
	serverAddr := getEnv("LONGCLAW_ISO_ADDR", "127.0.0.1:8443")
	fmt.Println("Running a TLS client", certPath, serverAddr)

	// Simulated issuer think time before each reply
	replyDelayMs, err := strconv.Atoi(getEnv("LONGCLAW_CLIENT_DELAY_MS", "3000"))
	if err != nil {
		log.Fatalf("invalid LONGCLAW_CLIENT_DELAY_MS: %v", err)
	}

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

	conn, err := tls.Dial("tcp", serverAddr, tlsConfig)
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

		// Reply as an issuer would: same message with the response MTI, so the server can match it by field 37
		respMTI, ok := responseMTI(string(message))
		if !ok {
			log.Println("Not a request message, not replying")
			continue
		}
		copy(message, respMTI)

		// Reply in the background so the delay does not hold up reading the next message.
		// tls.Conn serialises concurrent Writes, and each frame goes out in a single Write.
		go func() {
			time.Sleep(time.Millisecond * time.Duration(replyDelayMs))
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(n))
			buf = append(buf, message[0:n]...)
			log.Println("Writing data to ", conn.RemoteAddr())
			if _, err := conn.Write(buf); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
			log.Println("Wrote data ", string(message[0:n]))
		}()
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

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
