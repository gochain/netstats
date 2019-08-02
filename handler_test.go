package netstats_test

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gochain-io/netstats"
	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/websocket"
)

func TestHandler(t *testing.T) {
	t.Run("Init", func(t *testing.T) {
		s := NewServer()
		defer s.Close()

		s.DB.Trusted = netstats.GeoByIP{
			"127.0.0.1": &netstats.Geo{
				Range:   []int{1, 2},
				Country: "US",
				Region:  "CA",
				City:    "Santa Clara",
				LL:      []float64{1.23, 4.56},
				Metro:   100,
				Zip:     90210,
			},
		}

		// Connect to api and receive ready emit.
		apiConn := s.MustDial("/api")
		defer apiConn.Close()
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["hello",{"id":"node1","info":{"name":"node1","node":"GoChain/v1.0.0/linux-amd64/go1.0.0","port":30303,"net":"31337","protocol":"eth/63","api":"No","os":"linux","os_v":"amd64","client":"0.1.1","canUpdateHistory":true},"secret":"SECRET"}]}`)); err != nil {
			t.Fatal(err)
		} else if _, buf, err := apiConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if string(buf) != `{"emit":["ready"]}` {
			t.Fatalf("unexpected response: %s", buf)
		}

		// New connections should receive node list on connection.
		primusConn := s.MustDial("/primus")
		defer primusConn.Close()

		// Read initial "init" message from primus connection.
		if err := primusConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["ready"]}`)); err != nil {
			t.Fatal(err)
		} else if _, buf, err := primusConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if string(buf) != `{"emit":["init",{"nodes":[{"id":"node1","geo":{"city":"Santa Clara","country":"US","ll":[1.23,4.56],"metro":100,"range":[1,2],"region":"CA","zip":90210},"history":[-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1,-1],"info":{"api":"No","canUpdateHistory":true,"client":"0.1.1","ip":"127.0.0.1","name":"node1","net":"31337","node":"GoChain/v1.0.0/linux-amd64/go1.0.0","os":"linux","os_v":"amd64","port":30303,"protocol":"eth/63"},"stats":{"active":true,"mining":false,"syncing":false,"hashrate":0,"peers":0,"gasPrice":0,"uptime":100,"pending":0,"latency":"0","propagationAvg":0,"block":{"number":0,"hash":"0x0000000000000000000000000000000000000000000000000000000000000000","parentHash":"","timestamp":0,"miner":"","gasUsed":0,"gasLimit":0,"difficulty":"0","totalDifficulty":"0","transactions":[],"transactionCount":0,"transactionsRoot":"","stateRoot":"","uncles":[],"trusted":false,"arrived":0,"received":0,"propagation":0,"time":0,"fork":0}},"trusted":true,"uptime":{"down":0,"lastStatus":true,"lastUpdate":946684800000,"started":946684800000,"up":0}}]}]}`+"\n" {
			t.Fatalf("unexpected response: %s", buf)
		}
	})

	t.Run("Add", func(t *testing.T) {
		s := NewServer()
		defer s.Close()

		apiConn := s.MustDial("/api")
		defer apiConn.Close()

		primusConn := s.MustDial("/primus")
		defer primusConn.Close()

		// Send "ready" & read initial "init" message from primus connection.
		if err := primusConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["ready"]}`)); err != nil {
			t.Fatal(err)
		} else if _, buf, err := primusConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if string(buf) != `{"emit":["init",{"nodes":[]}]}`+"\n" {
			t.Fatalf("unexpected response: %s", buf)
		}

		// Connect to api and receive ready emit.
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["hello",{"id":"node1","info":{"name":"node1","node":"GoChain/v1.0.0/linux-amd64/go1.0.0","port":30303,"net":"31337","protocol":"eth/63","api":"No","os":"linux","os_v":"amd64","client":"0.1.1","canUpdateHistory":true},"secret":"SECRET"}]}`)); err != nil {
			t.Fatal(err)
		}

		// Read "add" message from primus connection.
		if _, buf, err := primusConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(string(buf), `{"action":"add","data":{"api":"No","canUpdateHistory":true,"client":"0.1.1","ip":"127.0.0.1","name":"node1","net":"31337","node":"GoChain/v1.0.0/linux-amd64/go1.0.0","os":"linux","os_v":"amd64","port":30303,"protocol":"eth/63"}}`); diff != "" {
			t.Fatalf(diff)
		}
	})

	t.Run("Block", func(t *testing.T) {
		s := NewServer()
		defer s.Close()

		// Connect to api.
		apiConn := s.MustDial("/api")
		defer apiConn.Close()
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["hello",{"id":"node1","secret":"SECRET"}]}`)); err != nil {
			t.Fatal(err)
		}

		// Connect to primus and skip "init" message.
		primusConn := s.MustDial("/primus")
		defer primusConn.Close()
		skipPrimusInit(t, primusConn)

		// Write pending message to API.
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["block",{"id":"node1","block":{"number":2270744,"hash":"0xa533690d4bb6bbedddf1288e0fa68e67767b3afcdfea3692200ea09fc6f26f0a","parentHash":"0x65d7d9a984cd4297df3a0d14c3ed040fdcf13ab654aa5602f8a203160c8aa6c9","timestamp":1541632732,"miner":"0x7aeceb5d345a01f8014a4320ab1f3d467c0c086a","gasUsed":20706000,"gasLimit":136500000,"difficulty":"5","totalDifficulty":"11833013","transactions":[{"hash":"0xd615b10f9e5349e1a2ffec4e3892558f3f3e4d2172a90011109def47e70d753b"},{"hash":"0xdb4d7fce7108c7a0eae39aa1154081440c28e533e288d983c136f2806961e30a"}],"transactionCount":2,"transactionsRoot":"0x15208174da18e98037f3ada4a8ff6f14cf485ad057cd851949712003340d3e58","stateRoot":"0x8124422cf11e2a84d67f16193c65dc7fc3dab3944d5257e4f90c1d2becc2aa27","uncles":[],"trusted":true,"arrived":1541632732569,"received":1541632733598,"propagation":1029,"fork":1}}]}`)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Pending", func(t *testing.T) {
		s := NewServer()
		defer s.Close()

		// Connect to api.
		apiConn := s.MustDial("/api")
		defer apiConn.Close()
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["hello",{"id":"node1","secret":"SECRET"}]}`)); err != nil {
			t.Fatal(err)
		}

		// Connect to primus and skip "init" message.
		primusConn := s.MustDial("/primus")
		defer primusConn.Close()
		skipPrimusInit(t, primusConn)

		// Write pending message to API.
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["pending",{"stats":{"pending":1000}}]}`)); err != nil {
			t.Fatal(err)
		}

		// Receive "pending" message for client.
		if _, buf, err := primusConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(string(buf), `{"action":"pending","data":{"id":"node1","pending":1000}}`); diff != "" {
			t.Fatalf(diff)
		}
	})

	t.Run("Stats", func(t *testing.T) {
		s := NewServer()
		defer s.Close()

		// Connect to api.
		apiConn := s.MustDial("/api")
		defer apiConn.Close()
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["hello",{"id":"node1","secret":"SECRET"}]}`)); err != nil {
			t.Fatal(err)
		}

		// Connect to primus and skip "init" message.
		primusConn := s.MustDial("/primus")
		defer primusConn.Close()
		skipPrimusInit(t, primusConn)

		// Write pending message to API.
		if err := apiConn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["stats",{"stats":{"active":true,"mining":false,"syncing":true,"hashrate":0,"peers":14,"gasPrice":2000000000,"uptime":100,"latency":"35"}}]}`)); err != nil {
			t.Fatal(err)
		}

		// Receive "pending" message for client.
		if _, buf, err := primusConn.ReadMessage(); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(string(buf), `{"action":"stats","data":{"id":"node1","stats":{"active":true,"mining":false,"syncing":true,"hashrate":0,"peers":14,"gasPrice":2000000000,"uptime":100,"pending":0,"latency":"0","propagationAvg":0}}}`); diff != "" {
			t.Fatalf(diff)
		}
	})
}

// Server represents a test wrapper for netstats.Server.
type Server struct {
	*httptest.Server
	Handler *netstats.Handler
	DB      *DB
}

func NewServer() *Server {
	s := &Server{Handler: netstats.NewHandler(), DB: NewDB()}
	s.Handler.DB = s.DB.DB
	s.Handler.APISecrets = []string{"SECRET"}
	s.Server = httptest.NewServer(s.Handler)
	return s
}

func (s *Server) MustDial(path string) *websocket.Conn {
	u := strings.Replace(s.URL+path, "http://", "ws://", -1)
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		panic(err)
	}
	return conn
}

// skipPrimusInit reads and skips over the emit/init message.
func skipPrimusInit(tb testing.TB, conn *websocket.Conn) {
	tb.Helper()

	// Send "ready" message & read message and ensure it is an emit/init message.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"emit":["ready"]}`)); err != nil {
		tb.Fatal(err)
	} else if _, buf, err := conn.ReadMessage(); err != nil {
		tb.Fatal(err)
	} else if !bytes.HasPrefix(buf, []byte(`{"emit":["init"`)) {
		tb.Fatalf("expected emit/init, got: %s", buf)
	}
}
