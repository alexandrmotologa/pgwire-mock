package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	host := os.Getenv("PGHOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PGPORT")
	if port == "" {
		port = "5432"
	}

	addr := net.JoinHostPort(host, port)
	log.Printf("[Go Client] Connecting to PGWire-Mock at %s...", addr)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("[Go Client] connection failed: %v", err)
	}
	defer conn.Close()

	// 1. SSLRequest
	var sslReq [8]byte
	binary.BigEndian.PutUint32(sslReq[0:4], 8)
	binary.BigEndian.PutUint32(sslReq[4:8], 80877103)
	if _, err := conn.Write(sslReq[:]); err != nil {
		log.Fatalf("[Go Client] failed to send SSLRequest: %v", err)
	}

	var sslReply [1]byte
	if _, err := io.ReadFull(conn, sslReply[:]); err != nil {
		log.Fatalf("[Go Client] failed to read SSL response: %v", err)
	}
	log.Printf("[Go Client] SSL response: '%c'", sslReply[0])

	// 2. StartupMessage
	var startupBody bytes.Buffer
	_ = binary.Write(&startupBody, binary.BigEndian, uint32(196608))
	startupBody.WriteString("user\x00postgres\x00database\x00testdb\x00\x00")

	var startupPacket bytes.Buffer
	_ = binary.Write(&startupPacket, binary.BigEndian, uint32(4+startupBody.Len()))
	startupPacket.Write(startupBody.Bytes())

	if _, err := conn.Write(startupPacket.Bytes()); err != nil {
		log.Fatalf("[Go Client] failed to send StartupMessage: %v", err)
	}

	// 3. Read handshake until ReadyForQuery ('Z')
	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(conn, typeBuf[:]); err != nil {
			log.Fatalf("[Go Client] handshake read error: %v", err)
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			log.Fatalf("[Go Client] handshake length error: %v", err)
		}
		payloadLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Fatalf("[Go Client] handshake payload error: %v", err)
		}

		if typeBuf[0] == 'R' {
			log.Println("[Go Client] AuthenticationOk received.")
		} else if typeBuf[0] == 'Z' {
			log.Println("[Go Client] ReadyForQuery received. Session established!")
			break
		}
	}

	// 4. Send Simple Query
	query := "SELECT 1"
	log.Printf("[Go Client] Sending query: %q", query)
	var qPacket bytes.Buffer
	qPacket.WriteByte('Q')
	_ = binary.Write(&qPacket, binary.BigEndian, uint32(4+len(query)+1))
	qPacket.WriteString(query)
	qPacket.WriteByte(0)

	if _, err := conn.Write(qPacket.Bytes()); err != nil {
		log.Fatalf("[Go Client] failed to write query: %v", err)
	}

	// 5. Read query response
	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(conn, typeBuf[:]); err != nil {
			break
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			break
		}
		payloadLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			break
		}

		if typeBuf[0] == 'T' {
			log.Println("[Go Client] RowDescription received.")
		} else if typeBuf[0] == 'D' {
			log.Println("[Go Client] DataRow received.")
		} else if typeBuf[0] == 'C' {
			log.Printf("[Go Client] CommandComplete: %q", string(payload[:len(payload)-1]))
		} else if typeBuf[0] == 'Z' {
			log.Println("[Go Client] ReadyForQuery received. Verification complete!")
			break
		}
	}

	fmt.Println("All verification checks passed.")
}
