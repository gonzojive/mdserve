package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"github.com/spf13/cobra"
)

// ChatMessage matches the server JSON chat protocol.
type ChatMessage struct {
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

var (
	chatMsg  string
	chatPort int
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Send messages to the WebTransport QUIC chat server",
	Long: `Connects to the mdserve WebTransport QUIC endpoint on port 8043 (or specified via flag)
to send a single message or start an interactive real-time chat in the terminal.

Example:
  devtool chat -m "Hello from CLI!"
  devtool chat (Starts interactive chat)
`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Configure WebTransport Dialer (with self-signed TLS override)
		dialer := &webtransport.Dialer{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"h3"},
			},
			QUICConfig: &quic.Config{
				EnableDatagrams:                  true,
				EnableStreamResetPartialDelivery: true,
			},
		}

		url := fmt.Sprintf("https://localhost:%d/webtransport", chatPort)
		log.Printf("Connecting to WebTransport endpoint: %s...", url)

		// Dial the server
		_, session, err := dialer.Dial(ctx, url, http.Header{})
		if err != nil {
			log.Fatalf("Failed to establish WebTransport session: %v", err)
		}
		defer session.CloseWithError(0, "client exit")

		// Open a persistent bidirectional stream
		stream, err := session.OpenStreamSync(ctx)
		if err != nil {
			log.Fatalf("Failed to open bidirectional stream: %v", err)
		}
		defer stream.Close()

		// If a single message was specified, send it and exit
		if chatMsg != "" {
			sendMsg(stream, "Devtool CLI", chatMsg)
			// Wait briefly for data to flush before exiting
			time.Sleep(100 * time.Millisecond)
			log.Println("Message sent successfully.")
			return
		}

		// Handle OS signals for clean shutdown in interactive mode
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			cancel()
			fmt.Println("\nExiting chat...")
			os.Exit(0)
		}()

		fmt.Println("==================================================")
		fmt.Printf(" Connected to QUIC Chat Server on port %d!\n", chatPort)
		fmt.Println(" Type your message and press Enter. Ctrl+C to exit.")
		fmt.Println("==================================================")

		// Goroutine to read incoming messages from the server
		go func() {
			reader := bufio.NewReader(stream)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("\n[Disconnected from server: %v]\n", err)
					cancel()
					return
				}

				var msg ChatMessage
				if err := json.Unmarshal([]byte(line), &msg); err == nil {
					// Format timestamp
					t, parseErr := time.Parse(time.RFC3339, msg.Timestamp)
					timeStr := msg.Timestamp
					if parseErr == nil {
						timeStr = t.Format("15:04:05")
					}

					// Don't echo back messages from "Devtool CLI" if we want a clean console feed,
					// but showing it is standard for chat UI.
					senderLabel := msg.Sender
					if msg.Sender == "Browser" {
						senderLabel = "\033[34mBrowser\033[0m" // Blue
					} else if msg.Sender == "Devtool CLI" {
						senderLabel = "\033[31mDevtool CLI\033[0m" // Red
					} else {
						senderLabel = "\033[35m" + msg.Sender + "\033[0m" // Magenta
					}

					fmt.Printf("\r[%s] %s: %s\n> ", timeStr, senderLabel, msg.Content)
				}
			}
		}()

		// Main thread: read user input from Stdin
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if !scanner.Scan() {
					return
				}
				text := strings.TrimSpace(scanner.Text())
				if text == "" {
					fmt.Print("> ")
					continue
				}

				sendMsg(stream, "Devtool CLI", text)
				fmt.Print("> ")
			}
		}
	},
}

func sendMsg(stream *webtransport.Stream, sender, content string) {
	msg := ChatMessage{
		Sender:    sender,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	payload = append(payload, '\n') // Newline delimiter for framing

	_, err = stream.Write(payload)
	if err != nil {
		log.Printf("Error writing to stream: %v", err)
	}
}

func init() {
	chatCmd.Flags().StringVarP(&chatMsg, "message", "m", "", "Single message content to send (non-interactive)")
	chatCmd.Flags().IntVarP(&chatPort, "port", "p", 8043, "WebTransport UDP port")
	rootCmd.AddCommand(chatCmd)
}
