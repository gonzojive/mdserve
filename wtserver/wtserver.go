package wtserver

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// ChatMessage represents the JSON structure of our chat protocol.
type ChatMessage struct {
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// StreamRegistry keeps track of all active bidirectional streams across WebTransport sessions.
type StreamRegistry struct {
	streams map[string]*webtransport.Stream
	mu      sync.Mutex
}

func newStreamRegistry() *StreamRegistry {
	return &StreamRegistry{
		streams: make(map[string]*webtransport.Stream),
	}
}

func (sr *StreamRegistry) Add(id string, stream *webtransport.Stream) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.streams[id] = stream
}

func (sr *StreamRegistry) Remove(id string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	delete(sr.streams, id)
}

// Hooks for message routing (implemented by wtserver/bridge.go)
var (
	OnMessage    func(streamID string, data []byte)
	OnDisconnect func(streamID string)
)

// SendToStream writes raw bytes to a specific stream.
func SendToStream(streamID string, data []byte) error {
	if registry == nil {
		return fmt.Errorf("server registry not initialized")
	}
	registry.mu.Lock()
	stream, exists := registry.streams[streamID]
	registry.mu.Unlock()
	if !exists {
		return fmt.Errorf("stream %s not found", streamID)
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, err := stream.Write(data)
	return err
}

// BroadcastRaw sends raw bytes to all connected streams.
func BroadcastRaw(data []byte) {
	if registry == nil {
		return
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for id, stream := range registry.streams {
		go func(sID string, s *webtransport.Stream) {
			s.Write(data)
		}(id, stream)
	}
}

// Global server references
var (
	registry *StreamRegistry
	wtServer *webtransport.Server
	certHash string // Hex-encoded SHA-256 hash of the self-signed cert DER bytes
)

// GetCertHash returns the hex-encoded SHA-256 fingerprint of the active TLS certificate.
func GetCertHash() string {
	return certHash
}

// UpgradeHandler upgrades HTTP requests to WebTransport QUIC sessions.
func UpgradeHandler(w http.ResponseWriter, r *http.Request) {
	if wtServer == nil {
		http.Error(w, "WebTransport server not initialized", http.StatusInternalServerError)
		return
	}

	// Upgrade connection
	session, err := wtServer.Upgrade(w, r)
	if err != nil {
		log.Printf("[WTSERVER] Upgrade failed: %v", err)
		return
	}

	go handleSession(session)
}

func handleSession(session *webtransport.Session) {
	log.Printf("[WTSERVER] WebTransport session accepted: %s", session.RemoteAddr().String())
	defer func() {
		log.Printf("[WTSERVER] WebTransport session closed: %s", session.RemoteAddr().String())
		session.CloseWithError(0, "session closed")
	}()

	// Loop to accept bidirectional streams
	for {
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			log.Printf("[WTSERVER] AcceptStream returned error: %v", err)
			break
		}
		go handleBidirectionalStream(session, stream)
	}
}

func handleBidirectionalStream(session *webtransport.Session, stream *webtransport.Stream) {
	streamID := fmt.Sprintf("%p", stream)
	log.Printf("[WTSERVER] Accepted bidirectional stream: %s", streamID)

	registry.Add(streamID, stream)
	defer func() {
		log.Printf("[WTSERVER] Bidirectional stream closed: %s", streamID)
		registry.Remove(streamID)
		if OnDisconnect != nil {
			OnDisconnect(streamID)
		}
		stream.Close()
	}()

	// Read lines (delimited by \n) in a loop
	reader := bufio.NewReader(stream)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break // Connection closed or read error
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if OnMessage != nil {
			OnMessage(streamID, []byte(line))
		}
	}
}

// InitAndStartServer generates the self-signed cert and starts the WebTransport server.
func InitAndStartServer(udpAddr string) error {
	registry = newStreamRegistry()

	// Generate certificate
	cert, hash, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to generate self-signed cert: %w", err)
	}
	certHash = hex.EncodeToString(hash[:])
	log.Printf("[WTSERVER] Generated TLS Certificate SHA-256 fingerprint: %s", certHash)

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	// Initialize WebTransport Server over HTTP/3
	wtServer = &webtransport.Server{
		H3: &http3.Server{
			Addr:            udpAddr,
			TLSConfig:       tlsConfig,
			EnableDatagrams: true,
			QUICConfig: &quic.Config{
				EnableDatagrams:                  true,
				EnableStreamResetPartialDelivery: true,
			},
		},
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow cross-origin connections for development
		},
	}

	webtransport.ConfigureHTTP3Server(wtServer.H3)

	// Configure default mux for HTTP/3 request handling
	mux := http.NewServeMux()
	mux.HandleFunc("/webtransport", UpgradeHandler)
	wtServer.H3.Handler = mux

	// Start server in a background thread
	go func() {
		log.Printf("[WTSERVER] Starting WebTransport (HTTP/3) UDP listener on %s", udpAddr)
		if err := wtServer.ListenAndServe(); err != nil {
			log.Printf("[WTSERVER] Server stopped: %v", err)
		}
	}()

	return nil
}

// generateSelfSignedCert generates an in-memory self-signed certificate valid for 10 days.
func generateSelfSignedCert() (tls.Certificate, [32]byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, [32]byte{}, err
	}

	// Validity must be < 14 days for WebTransport SHA-256 hash validation
	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 24 * time.Hour) // 10 days

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, [32]byte{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"MDServe Devtool Server"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, [32]byte{}, err
	}

	// Compute SHA-256 fingerprint
	hash := sha256.Sum256(derBytes)

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	return cert, hash, nil
}
