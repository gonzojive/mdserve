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
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"github.com/spf13/cobra"
)

// MCP JSON-RPC structs
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"` // Can be float64 or string
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolCallResult struct {
	Content []mcpTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// WebTransport Bridge envelope
type bridgeEnvelope struct {
	Type       string          `json:"type"`
	ClientType string          `json:"client_type,omitempty"`
	Id         string          `json:"id,omitempty"`
	Tool       string          `json:"tool,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

type toolResponsePayload struct {
	Success bool   `json:"success"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

var (
	mcpPort int
	// Map to track pending requests and their response channels
	pendingRequests = make(map[string]chan toolResponsePayload)
	pendingMu       sync.Mutex
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP stdio server to control the browser",
	Long: `Starts a Model Context Protocol (MCP) server over stdio.
It bridges the agent's tool calls to the browser via the mdserve WebTransport server.

Add this server to your local Claude Desktop config or equivalent:
{
  "mcpServers": {
    "mdserve-harness": {
      "command": "/home/red/bin/devtool",
      "args": ["mcp", "-p", "8043"]
    }
  }
}
`,
	Run: func(cmd *cobra.Command, args []string) {
		// Log errors to a file instead of Stderr/Stdout, as stdio is reserved for JSON-RPC
		logFile, err := os.OpenFile(os.TempDir()+"/mdserve-mcp.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(logFile)
		}
		log.Println("[MCP] Starting MCP stdio bridge...")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 1. Connect to WebTransport mdserve server
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

		url := fmt.Sprintf("https://localhost:%d/webtransport", mcpPort)
		log.Printf("[MCP] Dialing WebTransport server: %s", url)

		_, session, err := dialer.Dial(ctx, url, http.Header{})
		if err != nil {
			log.Printf("[MCP] Dial failed: %v", err)
			os.Exit(1)
		}
		defer session.CloseWithError(0, "")

		stream, err := session.OpenStreamSync(ctx)
		if err != nil {
			log.Printf("[MCP] Failed to open stream: %v", err)
			os.Exit(1)
		}
		defer stream.Close()

		// 2. Register as agent client on the WebTransport bridge
		regMsg := bridgeEnvelope{
			Type:       "register",
			ClientType: "agent",
		}
		regData, _ := json.Marshal(regMsg)
		regData = append(regData, '\n')
		stream.Write(regData)
		log.Println("[MCP] Registered client on bridge.")

		// 3. Start a goroutine to read WebTransport responses from the server
		go func() {
			reader := bufio.NewReader(stream)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					log.Printf("[MCP] WebTransport connection lost: %v", err)
					os.Exit(1)
				}

				var env bridgeEnvelope
				if err := json.Unmarshal([]byte(line), &env); err != nil {
					continue
				}

				if env.Type == "tool_response" {
					pendingMu.Lock()
					ch, exists := pendingRequests[env.Id]
					pendingMu.Unlock()

					if exists {
						var payload toolResponsePayload
						json.Unmarshal(env.Payload, &payload)
						ch <- payload
					}
				}
			}
		}()

		// 4. Read JSON-RPC requests from Stdin in a loop
		stdinReader := bufio.NewReader(os.Stdin)
		for {
			line, err := stdinReader.ReadString('\n')
			if err != nil {
				log.Printf("[MCP] Stdin closed: %v", err)
				break
			}

			var req jsonRPCRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				log.Printf("[MCP] Error unmarshaling JSON-RPC: %v", err)
				continue
			}

			handleJSONRPC(stream, req)
		}
	},
}

func handleJSONRPC(stream *webtransport.Stream, req jsonRPCRequest) {
	log.Printf("[MCP] Received request: method=%s, id=%v", req.Method, req.ID)

	var response jsonRPCResponse
	response.JSONRPC = "2.0"
	response.ID = req.ID

	switch req.Method {
	case "initialize":
		response.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "mdserve-harness",
				"version": "1.0.0",
			},
		}

	case "notifications/initialized":
		// Notification, no response needed
		return

	case "tools/list":
		response.Result = map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{
					"name":        "get_diagram",
					"description": "Get the current Mermaid diagram code rendered in the browser",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
				map[string]interface{}{
					"name":        "update_diagram",
					"description": "Overwrite the current diagram in the browser with new Mermaid code",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"code": map[string]interface{}{
								"type":        "string",
								"description": "The new Mermaid diagram code",
							},
						},
						"required": []string{"code"},
					},
				},
				map[string]interface{}{
					"name":        "add_node",
					"description": "Add a new node and an optional connection link to the current Mermaid diagram in the browser",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type":        "string",
								"description": "Unique ID of the new node (e.g. 'DB')",
							},
							"label": map[string]interface{}{
								"type":        "string",
								"description": "Display label of the new node (e.g. 'Database')",
							},
							"edgeTo": map[string]interface{}{
								"type":        "string",
								"description": "Optional ID of an existing node to connect this node from",
							},
						},
						"required": []string{"id", "label"},
					},
				},
				map[string]interface{}{
					"name":        "set_theme",
					"description": "Toggle the browser theme between light and dark mode",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"theme": map[string]interface{}{
								"type":        "string",
								"description": "Theme name",
								"enum":        []string{"light", "dark"},
							},
						},
						"required": []string{"theme"},
					},
				},
			},
		}

	case "tools/call":
		// Parse call params
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		json.Unmarshal(req.Params, &params)

		// Call the WebTransport bridge
		reqID := fmt.Sprintf("mcp-req-%d", time.Now().UnixNano())
		ch := make(chan toolResponsePayload, 1)

		pendingMu.Lock()
		pendingRequests[reqID] = ch
		pendingMu.Unlock()

		defer func() {
			pendingMu.Lock()
			delete(pendingRequests, reqID)
			pendingMu.Unlock()
		}()

		// Wrap and send tool request
		env := bridgeEnvelope{
			Type:    "tool_request",
			Id:      reqID,
			Tool:    params.Name,
			Payload: params.Arguments,
		}
		data, _ := json.Marshal(env)
		data = append(data, '\n')
		stream.Write(data)

		// Wait for response with a 5 second timeout
		select {
		case resp := <-ch:
			if resp.Success {
				response.Result = mcpToolCallResult{
					Content: []mcpTextContent{
						{Type: "text", Text: resp.Result},
					},
				}
			} else {
				response.Result = mcpToolCallResult{
					Content: []mcpTextContent{
						{Type: "text", Text: resp.Error},
					},
					IsError: true,
				}
			}
		case <-time.After(5 * time.Second):
			response.Result = mcpToolCallResult{
				Content: []mcpTextContent{
					{Type: "text", Text: "Error: Request to browser client timed out after 5 seconds."},
				},
				IsError: true,
			}
		}

	default:
		response.Error = map[string]interface{}{
			"code":    -32601,
			"message": fmt.Sprintf("Method %s not found", req.Method),
		}
	}

	// Write JSON-RPC response back to stdout
	respBytes, _ := json.Marshal(response)
	respBytes = append(respBytes, '\n')
	os.Stdout.Write(respBytes)
}

func init() {
	mcpCmd.Flags().IntVarP(&mcpPort, "port", "p", 8043, "WebTransport UDP port")
	rootCmd.AddCommand(mcpCmd)
}
