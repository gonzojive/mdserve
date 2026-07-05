package wtserver

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// BridgeEnvelope is the generic wrapper for all messages routed through the WebTransport server.
type BridgeEnvelope struct {
	Type       string          `json:"type"`                  // "register", "tool_request", "tool_response", "chat"
	ClientType string          `json:"client_type,omitempty"` // "browser", "agent"
	Id         string          `json:"id,omitempty"`          // Request ID for matching responses to requests
	Tool       string          `json:"tool,omitempty"`        // Tool name (e.g. "update_diagram")
	Payload    json.RawMessage `json:"payload,omitempty"`     // Raw JSON payload for the specific message type
}

// Bridge manages the routing of tool calls and chat messages between browser sessions and agent/CLI sessions.
type Bridge struct {
	browsers map[string]bool
	agents   map[string]bool
	pending  map[string]string // requestID -> agentStreamID
	mu       sync.Mutex
}

var bridge *Bridge

func init() {
	bridge = &Bridge{
		browsers: make(map[string]bool),
		agents:   make(map[string]bool),
		pending:  make(map[string]string),
	}

	// Register the bridge as the global WebTransport message handler
	OnMessage = bridge.HandleMessage
	OnDisconnect = bridge.HandleDisconnect
}

// HandleDisconnect cleans up registered streams when a client disconnects.
func (b *Bridge) HandleDisconnect(streamID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browsers[streamID] {
		delete(b.browsers, streamID)
		log.Printf("[BRIDGE] Unregistered Browser stream: %s", streamID)
	}
	if b.agents[streamID] {
		delete(b.agents, streamID)
		log.Printf("[BRIDGE] Unregistered Agent/CLI stream: %s", streamID)
	}
}

// HandleMessage parses and routes the JSON envelope to the appropriate recipient.
func (b *Bridge) HandleMessage(streamID string, data []byte) {
	var env BridgeEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		log.Printf("[BRIDGE] Error parsing bridge envelope: %v", err)
		return
	}

	switch env.Type {
	case "register":
		b.mu.Lock()
		if env.ClientType == "browser" {
			b.browsers[streamID] = true
			log.Printf("[BRIDGE] Registered Browser stream: %s", streamID)
		} else if env.ClientType == "agent" {
			b.agents[streamID] = true
			log.Printf("[BRIDGE] Registered Agent/CLI stream: %s", streamID)
		}
		b.mu.Unlock()

	case "tool_request":
		b.mu.Lock()
		b.pending[env.Id] = streamID // map request ID to originating agent stream
		browserStreamID := b.getActiveBrowser()
		b.mu.Unlock()

		if browserStreamID == "" {
			log.Printf("[BRIDGE] Error: No active browser connection found to execute tool '%s'", env.Tool)
			// Return error response back to agent
			b.sendErrorResponse(env.Id, streamID, "No active browser session found")
			return
		}

		log.Printf("[BRIDGE] Routing tool request '%s' (%s) from Agent %s to Browser %s", env.Tool, env.Id, streamID, browserStreamID)
		if err := SendToStream(browserStreamID, data); err != nil {
			log.Printf("[BRIDGE] Failed to route request to browser: %v", err)
			b.sendErrorResponse(env.Id, streamID, "Failed to send command to browser client")
		}

	case "tool_response":
		b.mu.Lock()
		agentStreamID := b.pending[env.Id]
		delete(b.pending, env.Id) // clear pending mapping
		b.mu.Unlock()

		if agentStreamID == "" {
			log.Printf("[BRIDGE] Error: Originating agent stream for request '%s' not found", env.Id)
			return
		}

		log.Printf("[BRIDGE] Routing tool response (%s) back to Agent %s", env.Id, agentStreamID)
		if err := SendToStream(agentStreamID, data); err != nil {
			log.Printf("[BRIDGE] Failed to route response back to agent: %v", err)
		}

	case "chat":
		// Handle legacy chat functionality: broadcast message to all connected streams
		log.Printf("[BRIDGE] Broadcasting chat message from stream %s...", streamID)
		BroadcastRaw(data)

	default:
		log.Printf("[BRIDGE] Unknown message type received: %s", env.Type)
	}
}

func (b *Bridge) getActiveBrowser() string {
	// Simple strategy: return the first browser in the map.
	// In production, this can support selecting the active tab or session.
	for id := range b.browsers {
		return id
	}
	return ""
}

func (b *Bridge) sendErrorResponse(reqID, agentStreamID, errMsg string) {
	resp := BridgeEnvelope{
		Type: "tool_response",
		Id:   reqID,
		Payload: json.RawMessage(fmt.Sprintf(`{"success":false,"error":%q}`, errMsg)),
	}
	data, _ := json.Marshal(resp)
	SendToStream(agentStreamID, data)
}

// helper inline import fix if fmt not imported
