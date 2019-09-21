package netstats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gochain-io/netstats/assets"

	"github.com/gorilla/websocket"
	"github.com/tomasen/realip"
	"go.uber.org/zap"
)

const (
	HistoryRequestInterval = int64((2 * time.Minute) / time.Millisecond)
)

type Handler struct {
	lgr *zap.Logger

	mu         sync.RWMutex
	conns      map[*Conn]struct{} // primus connections
	fileServer http.Handler

	// Underlying database.
	DB *DB

	// Secret used to authorize node.
	APISecrets []string
}

// NewHandler returns a new instance of Handler.
func NewHandler(lgr *zap.Logger) *Handler {
	return &Handler{
		lgr:        lgr,
		conns:      make(map[*Conn]struct{}),
		fileServer: assets.FileServer(),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r.URL.Path = "/index.html"
	}

	switch strings.TrimSuffix(r.URL.Path, "/") {
	case "/api":
		h.handleAPI(w, r)
	case "/external":
		h.handleExternal(w, r)
	case "/primus":
		h.handlePrimus(w, r)
	case "/debug/nodes":
		h.handleDebugNodes(w, r)
	case "/debug/blocks":
		h.handleDebugBlocks(w, r)
	default:
		h.fileServer.ServeHTTP(w, r)
	}
}

// handleAPI handles incoming node requests.
func (h *Handler) handleAPI(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrade(w, r)
	if err != nil {
		h.lgr.Error("API: upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	var nodeID string
	var authorized bool
	var lastHistoryRequestTime int64

	// Ensure node is marked as inactive when the connection is lost.
	defer func() {
		if nodeID == "" {
			return
		} else if err := h.DB.SetInactive(r.Context(), nodeID); err != nil {
			h.lgr.Error("API: failed to set inactive", zap.String("id", nodeID), zap.Error(err))
			return
		}

		node, err := h.DB.FindNodeByID(r.Context(), nodeID)
		if err != nil {
			h.lgr.Error("API: failed to find node", zap.String("id", nodeID), zap.Error(err))
			return
		}
		h.publish("inactive", node.Stats)
	}()

	for {
		var msg APIMessage
		if _, buf, err := conn.ReadMessage(); IsUnexpectedCloseError(err) {
			return
		} else if err != nil {
			h.lgr.Error("API: failed to read message", zap.Error(err))
			return
		} else if err := json.Unmarshal(buf, &msg); err != nil {
			h.lgr.Error("API: failed to unmarshal message", zap.Error(err))
			return
		} else if len(msg.Emit) == 0 {
			h.lgr.Error("API: empty emit, exiting", zap.ByteString("buf", buf))
			return
		}

		// End connection it has not been authorized yet.
		var action string
		if err := json.Unmarshal(msg.Emit[0], &action); err != nil {
			h.lgr.Error("API: failed to unmarshal action (emit[0])", zap.Error(err))
			return
		} else if action != "hello" && !authorized {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"reconnect":false}`))
			return
		}

		// Fetch data if set as second argument.
		var dataBuf []byte
		if len(msg.Emit) > 1 {
			dataBuf = []byte(msg.Emit[1])
		}

		lgr := h.lgr.With(zap.String("action", action))
		if nodeID != "" {
			lgr = lgr.With(zap.String("id", nodeID))
		}

		switch action {
		case "hello":
			var data HelloMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal data", zap.Error(err))
				return
			} else if !h.IsValidAPISecret(data.Secret) {
				conn.WriteMessage(websocket.TextMessage, []byte(`{"reconnect":false}`))
				return
			}
			authorized = true
			nodeID = data.ID

			if data.Info == nil {
				data.Info = &NodeInfo{}
			}

			node := NewNode(h.DB.Now())
			node.ID = nodeID
			node.Info = data.Info
			node.Info.IP = realip.FromRequest(r)
			node.Stats.Latency = data.Latency

			if err := h.DB.CreateNodeIfNotExists(r.Context(), node); err != nil {
				lgr.Error("API: failed to add node", zap.String("action", action), zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.String("action", action), zap.Error(err))
				return
			}

			h.publish("add", node.Info)

			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["ready"]}`)); IsUnexpectedCloseError(err) {
				return
			} else if err != nil {
				lgr.Error("API: failed to emit ready", zap.String("action", action), zap.Error(err))
				return
			}

		case "update":
			var data UpdateMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			} else if err := h.DB.AddBlock(r.Context(), nodeID, data.Stats.Block); err != nil {
				lgr.Error("API: failed to add block:", zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.Error(err))
				return
			}
			h.publish("update", node.Info)

		case "block":
			var data BlockMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			} else if err := h.DB.AddBlock(r.Context(), nodeID, data.Block); err != nil {
				lgr.Error("API: failed to add block", zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.Error(err))
				return
			}
			h.publish("block", map[string]interface{}{
				"id":             nodeID,
				"block":          node.Stats.Block,
				"history":        node.History,
				"propagationAvg": node.Stats.PropagationAvg,
			})

		case "pending":
			var data PendingMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal pending data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			} else if err := h.DB.UpdatePending(r.Context(), nodeID, data.Stats.Pending); err != nil {
				lgr.Error("API: failed to update pending", zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.Error(err))
				return
			}
			h.publish("pending", map[string]interface{}{"id": nodeID, "pending": node.Stats.Pending})

		case "stats":
			var data StatsMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal stats data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			} else if err := h.DB.UpdateStats(r.Context(), nodeID, data.Stats); err != nil {
				lgr.Error("API: failed to update stats", zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.Error(err))
				return
			}

			var stats Stats
			stats.Active = node.Stats.Active
			stats.Mining = node.Stats.Mining
			stats.Syncing = node.Stats.Syncing
			stats.Hashrate = node.Stats.Hashrate
			stats.Peers = node.Stats.Peers
			stats.GasPrice = node.Stats.GasPrice
			stats.Uptime = node.Stats.Uptime
			stats.Latency = node.Stats.Latency

			h.publish("stats", map[string]interface{}{"id": nodeID, "stats": stats})

		case "history":
			var data HistoryMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal history data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			}

			if err := h.DB.AddBlocks(r.Context(), nodeID, data.History); err != nil {
				lgr.Error("API: failed to add blocks", zap.Error(err))
				return
			}

			node, err := h.DB.FindNodeByID(r.Context(), nodeID)
			if err != nil {
				lgr.Error("API: failed to find node", zap.Error(err))
				return
			}
			h.publish("history", node.History)

		case "node-ping":
			var data NodePingMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			}

			if err := emit(conn, "node-pong", NodePongMessageData{}); err != nil {
				lgr.Error("API: failed to emit node-pong", zap.Error(err))
				return
			}

		case "latency":
			var data LatencyMessageData
			if err := json.Unmarshal(dataBuf, &data); err != nil {
				lgr.Error("API: failed to unmarshal data", zap.ByteString("data", dataBuf), zap.Error(err))
				return
			} else if err := h.DB.UpdateLatency(r.Context(), nodeID, data.Latency); err != nil {
				lgr.Error("API: failed to update latency", zap.Error(err))
				return
			}

			// Request history if not requested recently.
			maxBlockNumber := h.DB.MaxBlockNumber(r.Context())
			if d := h.DB.Now() - lastHistoryRequestTime; d > HistoryRequestInterval && maxBlockNumber > 0 {
				lastHistoryRequestTime = h.DB.Now()

				minBlockNumber := maxBlockNumber - MaxHistory
				if minBlockNumber < 0 {
					minBlockNumber = 0
				}
				if err := emit(conn, "history", RequestHistoryMessageData{
					Max:  maxBlockNumber,
					Min:  minBlockNumber,
					List: reverseIntSliceRange(maxBlockNumber, minBlockNumber),
				}); IsUnexpectedCloseError(err) {
					return
				} else if err != nil {
					lgr.Error("API: failed to emit history request", zap.Error(err))
					return
				}
			}

		default:
			lgr.Error("API: unknown action", zap.String("action", action))
			return
		}
	}
}

// handleExternal handles the external API.
func (h *Handler) handleExternal(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrade(w, r)
	if err != nil {
		h.lgr.Error("EXTERNAL: failed to upgrade", zap.Error(err))
		return
	}
	defer conn.Close()

	notify := h.DB.BestBlockNumberNotify()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-notify:
			bestBlockNumber := h.DB.BestBlockNumber(r.Context())
			if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"action":"lastBlock","number":%d}`, bestBlockNumber))); IsUnexpectedCloseError(err) {
				continue
			} else if err != nil {
				h.lgr.Error("EXTERNAL: failed to write best block message", zap.Int("blockNumber", bestBlockNumber), zap.Error(err))
				return
			}

			notify = h.DB.BestBlockNumberNotify()
			return
		}
	}
}

// handlePrimus handles incoming web client requests.
func (h *Handler) handlePrimus(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrade(w, r)
	if err != nil {
		h.lgr.Error("PRIMUS: failed to upgrade", zap.Error(err))
		return
	}
	defer conn.Close()

	ready := make(chan struct{})
	go h.handlePrimusIncoming(conn, ready)

	// Wait for "ready" message from client.
	select {
	case <-r.Context().Done():
		return
	case <-time.After(5 * time.Second):
		return
	case <-ready:
	}

	// Fetch initial state of all nodes.
	nodes, err := h.DB.Nodes(r.Context())
	if err != nil {
		h.lgr.Error("PRIMUS: failed to fetch nodes", zap.Error(err))
		return
	}

	// Emit "init" with all nodes after receiving a "ready" from client.
	if err := conn.WriteJSON(&Message{Emit: []interface{}{"init", map[string]interface{}{"nodes": nodes}}}); IsUnexpectedCloseError(err) {
		return
	} else if err != nil {
		h.lgr.Error("PRIMUS: failed to write init JSON", zap.Error(err))
		return
	}

	// Attach connection to receive all node updates.
	h.subscribe(conn)
	defer h.unsubscribe(conn)

	// Emit "client-ping" with timestamp every 5s, "chart" when chart changes.
	chartNotify := h.DB.ChartNotify()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := conn.WriteJSON(map[string]interface{}{
				"action": "client-ping",
				"data": map[string]interface{}{
					"serverTime": h.DB.Now(),
				},
			}); IsUnexpectedCloseError(err) {
				return
			} else if err != nil {
				h.lgr.Error("PRIMUS: failed to write JSON", zap.String("action", "client-ping"), zap.Error(err))
				return
			}
		case <-chartNotify:
			if err := conn.WriteJSON(map[string]interface{}{
				"action": "charts",
				"data":   h.DB.Chart(),
			}); IsUnexpectedCloseError(err) {
				return
			} else if err != nil {
				h.lgr.Error("PRIMUS: failed to write JSON", zap.String("action", "charts"), zap.Error(err))
				return
			}
			time.Sleep(1 * time.Second)
			chartNotify = h.DB.ChartNotify()
		}
	}
}

// handlePrimusIncoming handles incoming pings & emits from primus.
func (h *Handler) handlePrimusIncoming(conn *Conn, ready chan struct{}) {
	var readyNotified bool
	for {
		_, buf, err := conn.ReadMessage()
		if IsUnexpectedCloseError(err) {
			return
		} else if err != nil {
			h.lgr.Error("PRIMUS: failed to read message", zap.Error(err))
			return
		}

		// Handle plaintext "primus:ping".
		if bytes.HasPrefix(buf, []byte(`"primus::ping::`)) {
			timestamp := bytes.TrimSuffix(bytes.TrimPrefix(buf, []byte(`"primus::ping::`)), []byte(`"`))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`"primus::pong::%s"`, timestamp))); IsUnexpectedCloseError(err) {
				return
			} else if err != nil {
				h.lgr.Error("PRIMUS: failed to write pong message", zap.Error(err))
				continue
			}
			continue
		}

		var msg Message
		d := json.NewDecoder(bytes.NewReader(buf))
		d.UseNumber()
		if err := d.Decode(&msg); err != nil {
			h.lgr.Error("PRIMUS: failed to unmarshal message", zap.Error(err))
			continue
		}

		// Handle primus-emit calls.
		if len(msg.Emit) > 0 {
			action, _ := msg.Emit[0].(string)
			switch action {
			case "ready":
				if !readyNotified {
					close(ready)
					readyNotified = true
				}
			case "client-pong":
				if len(msg.Emit) > 1 {
					data := msg.Emit[1].(map[string]interface{})
					prevServerTime, err := data["serverTime"].(json.Number).Int64()
					if err != nil {
						h.lgr.Error("PRIMUS: invalid serverTime field", zap.String("action", "client-pong"), zap.Error(err))
					}
					latency := ClientLatencyMessageData{Latency: (h.DB.Now() - prevServerTime) / 2}
					if err := emit(conn, "client-latency", latency); err != nil {
						h.lgr.Error("PRIMUS: failed to emit client-latency message", zap.Error(err))
						return
					}
				}
			default:
				h.lgr.Error("PRIMUS: unknown action", zap.String("action", action))
			}
			continue
		}
	}
}

func (h *Handler) handleDebugNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.DB.Nodes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buf, _ := json.MarshalIndent(nodes, "", "  ")
	w.Write(buf)
}

func (h *Handler) handleDebugBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.DB.Blocks(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buf, _ := json.MarshalIndent(blocks, "", "  ")
	w.Write(buf)
}

func (h *Handler) IsValidAPISecret(s string) bool {
	for _, secret := range h.APISecrets {
		if s == secret {
			return true
		}
	}
	return false
}

func (h *Handler) subscribe(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn] = struct{}{}
}

func (h *Handler) unsubscribe(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, conn)
}

func (h *Handler) publish(action string, data interface{}) {
	msg := &PublishMessage{Action: action, Data: data}
	buf, err := json.Marshal(msg)
	if err != nil {
		h.lgr.Error("API: failed to marshal PublishMessage", zap.String("action", action), zap.Error(err))
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.conns {
		if err := conn.WriteMessage(websocket.TextMessage, buf); IsUnexpectedCloseError(err) {
			continue
		} else if err != nil {
			h.lgr.Error("API: failed to write PublishMessage", zap.String("action", action), zap.Error(err))
			conn.Close()
			delete(h.conns, conn)
			continue
		}
	}
}

func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	var upgrader websocket.Upgrader
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return &Conn{Conn: conn}, nil
}

// Conn is a websocket connection with an attached mutex.
type Conn struct {
	mu sync.Mutex
	*websocket.Conn
}

func (c *Conn) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(messageType, data)
}

func (c *Conn) WriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(v)
}

type Message struct {
	Action         string          `json:"action,omitempty"`
	PropagationAvg uint64          `json:"propagationAvg,omitempty"`
	History        []int64         `json:"history,omitempty"`
	Emit           []interface{}   `json:"emit,omitempty"` // primus-emit
	Data           json.RawMessage `json:"data,omitempty"`
}

type APIMessage struct {
	Emit []json.RawMessage `json:"emit,omitempty"`
}

type HelloMessageData struct {
	ID      string    `json:"id"`
	Secret  string    `json:"secret"`
	Info    *NodeInfo `json:"info"`
	Latency int64     `json:"latency,string"`
}

type UpdateMessageData struct {
	ID    string `json:"id"`
	Stats *Stats `json:"stats"`
}

type BlockMessageData struct {
	ID    string `json:"id"`
	Block *Block `json:"block"`
}

type PendingMessageData struct {
	ID    string `json:"id"`
	Stats *Stats `json:"stats"`
}

type StatsMessageData struct {
	ID    string `json:"id"`
	Stats Stats  `json:"stats"`
}

type HistoryMessageData struct {
	ID      string   `json:"id"`
	History []*Block `json:"history"`
}

type LatencyMessageData struct {
	ID      string `json:"id"`
	Latency int64  `json:"latency,string"`
}

type RequestHistoryMessageData struct {
	Max  int   `json:"max"`
	Min  int   `json:"min"`
	List []int `json:"list"`
}

type ClientPingMessageData struct {
	ServerTime int64 `json:"serverTime"`
}

type ClientLatencyMessageData struct {
	Latency int64 `json:"latency"`
}

type NodePingMessageData struct{}

type NodePongMessageData struct{}

type PublishMessage struct {
	Action string      `json:"action"`
	Data   interface{} `json:"data"`
}

func emit(conn *Conn, args ...interface{}) error {
	return conn.WriteJSON(map[string]interface{}{"emit": args})
}

func reverseIntSliceRange(max, min int) []int {
	assert(max >= min, "reverseIntSliceRange: max must be greater than min")
	other := make([]int, 0, max-min)
	for i := max; i >= min; i-- {
		other = append(other, i)
	}
	return other
}

func IsUnexpectedCloseError(err error) bool {
	return err != nil && (websocket.IsUnexpectedCloseError(err) || strings.Contains(err.Error(), "broken pipe"))
}
