package netstats_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/gochain-io/netstats"
	"github.com/google/go-cmp/cmp"
)

func TestDB_FindNodeByID(t *testing.T) {
	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if _, err := db.FindNodeByID(ctx, "X"); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_CreateNodeIfNotExists(t *testing.T) {
	// Ensure an unregistered IP is marked as untrusted.
	t.Run("Untrusted", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}
		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:    "X",
			Geo:   &netstats.Geo{LL: []float64{0, 0}},
			Info:  &netstats.NodeInfo{},
			Stats: &netstats.Stats{Active: true, Uptime: 100},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	// Ensure a registered IP is marked as trusted and geolocation is updated.
	t.Run("Trusted", func(t *testing.T) {
		db := NewDB()
		db.GeoByIP["127.0.0.1"] = &netstats.Geo{City: "New York", Country: "USA"}
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{
			ID:   "X",
			Info: &netstats.NodeInfo{IP: "127.0.0.1"},
		}); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Trusted: true,
			Geo:     &netstats.Geo{City: "New York", Country: "USA"},
			Info:    &netstats.NodeInfo{IP: "127.0.0.1"},
			Stats:   &netstats.Stats{Active: true, Uptime: 100},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	// Ensure that readding a node updates its status.
	t.Run("AlreadyExists", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(time.Second)
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:    "X",
			Geo:   &netstats.Geo{LL: []float64{0, 0}},
			Info:  &netstats.NodeInfo{},
			Stats: &netstats.Stats{Active: true, Uptime: 100},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684801000,
				Started:    946684800000,
				Up:         1000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})
}

func TestDB_UpdatePending(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		} else if err := db.UpdatePending(ctx, "X", 1000); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node.Stats, &netstats.Stats{
			Active:  true,
			Pending: 1000,
			Uptime:  100,
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.UpdatePending(ctx, "X", 1000); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_UpdateStats(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		} else if err := db.UpdateStats(ctx, "X", netstats.Stats{
			Active:   false,
			Mining:   true,
			Syncing:  true,
			Hashrate: 40,
			Peers:    50,
			GasPrice: 60,
			Uptime:   70,
		}); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node.Stats, &netstats.Stats{
			Active:   false,
			Mining:   true,
			Syncing:  true,
			Hashrate: 40,
			Peers:    50,
			GasPrice: 60,
			Uptime:   70,
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.UpdateStats(ctx, "X", netstats.Stats{}); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_AddBlock(t *testing.T) {
	t.Run("InitialBlock", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		} else if err := db.AddBlock(ctx, "X", &netstats.Block{
			Hash:       "0x0000",
			ParentHash: "0x0001",
		}); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 0},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
				Block: &netstats.Block{
					Arrived:      946684800000,
					Hash:         "0x0000",
					ParentHash:   "0x0001",
					Received:     946684800000,
					Transactions: []*netstats.Transaction{},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("Readd", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Hash:       "0x0000",
			ParentHash: "0x0001",
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Hash:       "0x0000",
			ParentHash: "0x0001",
		}); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 0},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
				Block: &netstats.Block{
					Arrived:      946684800000,
					Hash:         "0x0000",
					ParentHash:   "0x0001",
					Received:     946684800000,
					Transactions: []*netstats.Transaction{},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("MultipleBlocks", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Number:          1,
			TotalDifficulty: 10,
			Hash:            "0x0001",
			ParentHash:      "0x0000",
			Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
		}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(1 * time.Second)
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Number:          2,
			TotalDifficulty: 20,
			Hash:            "0x0002",
			ParentHash:      "0x0001",
			Transactions:    []*netstats.Transaction{{Hash: "0x000C"}, {Hash: "0x000D"}},
		}); err != nil {
			t.Fatal(err)
		}

		notify := db.BestBlockNumberNotify()

		db.Now = db.Now.Add(1 * time.Second)
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Number:          3,
			TotalDifficulty: 30,
			Hash:            "0x0003",
			ParentHash:      "0x0002",
			Transactions:    []*netstats.Transaction{{Hash: "0x000E"}, {Hash: "0x000F"}},
		}); err != nil {
			t.Fatal(err)
		}

		// Ensure best block number notification occurs.
		select {
		case <-notify:
		default:
			t.Fatal("expected best block number notification")
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 0, 0, 0},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
				Block: &netstats.Block{
					Number:          3,
					TotalDifficulty: 30,
					Hash:            "0x0003",
					Arrived:         946684802000,
					Received:        946684802000,
					ParentHash:      "0x0002",
					Transactions:    []*netstats.Transaction{{Hash: "0x000E"}, {Hash: "0x000F"}},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("MultipleNodes", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		} else if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "Y"}); err != nil {
			t.Fatal(err)
		} else if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "Z"}); err != nil {
			t.Fatal(err)
		}

		// Add block#1 for X.
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Number:          1,
			TotalDifficulty: 10,
			Hash:            "0x0001",
			ParentHash:      "0x0000",
			Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
		}); err != nil {
			t.Fatal(err)
		}

		// Add block#1 for Y.
		db.Now = db.Now.Add(1 * time.Second)
		if err := db.AddBlock(ctx, "Y", &netstats.Block{
			Number:          1,
			TotalDifficulty: 10,
			Hash:            "0x0001",
			ParentHash:      "0x0000",
			Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
		}); err != nil {
			t.Fatal(err)
		}

		// Add block#2 for X.
		db.Now = db.Now.Add(1 * time.Second)
		if err := db.AddBlock(ctx, "X", &netstats.Block{
			Number:          2,
			TotalDifficulty: 20,
			Hash:            "0x0002",
			ParentHash:      "0x0001",
			Transactions:    []*netstats.Transaction{{Hash: "0x000C"}, {Hash: "0x000D"}},
		}); err != nil {
			t.Fatal(err)
		}

		// Add block#1 for Z.
		db.Now = db.Now.Add(1 * time.Second)
		if err := db.AddBlock(ctx, "Z", &netstats.Block{
			Number:          1,
			TotalDifficulty: 10,
			Hash:            "0x0001",
			ParentHash:      "0x0000",
			Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
		}); err != nil {
			t.Fatal(err)
		}

		// Verify X node status.
		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 0, 0},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
				Block: &netstats.Block{
					Number:          2,
					TotalDifficulty: 20,
					Hash:            "0x0002",
					Arrived:         946684802000,
					Received:        946684802000,
					ParentHash:      "0x0001",
					Transactions:    []*netstats.Transaction{{Hash: "0x000C"}, {Hash: "0x000D"}},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}

		// Verify Y node status.
		if node, err := db.FindNodeByID(ctx, "Y"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "Y",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 1000},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active:         true,
				PropagationAvg: 1000,
				Uptime:         100,
				Block: &netstats.Block{
					Number:          1,
					TotalDifficulty: 10,
					Hash:            "0x0001",
					Arrived:         946684801000,
					Received:        946684801000,
					Propagation:     1000,
					ParentHash:      "0x0000",
					Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}

		// Verify Z node status.
		if node, err := db.FindNodeByID(ctx, "Z"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "Z",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 3000, -1},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active:         true,
				PropagationAvg: 3000,
				Uptime:         100,
				Block: &netstats.Block{
					Number:          1,
					TotalDifficulty: 10,
					Hash:            "0x0001",
					Arrived:         946684803000,
					Received:        946684803000,
					Propagation:     -1,
					ParentHash:      "0x0000",
					Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ExceedMaxHistory", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		for i := 1; i <= netstats.MaxHistory+10; i++ {
			db.Now = db.Now.Add(time.Second)

			if err := db.AddBlock(ctx, "X", &netstats.Block{
				Number:     i,
				Hash:       fmt.Sprintf("0x%04x", i),
				ParentHash: fmt.Sprintf("0x%04x", i+1),
			}); err != nil {
				t.Fatal(err)
			}
		}

		if v := db.BestBlockNumber(ctx); v != 50 {
			t.Fatalf("unexpected best block number: %d", v)
		} else if v := db.MaxBlockNumber(ctx); v != 50 {
			t.Fatalf("unexpected max block number: %d", v)
		} else if v := db.MinBlockNumber(ctx); v != 10 {
			t.Fatalf("unexpected min block number: %d", v)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:      "X",
			Geo:     &netstats.Geo{LL: []float64{0, 0}},
			History: []int64{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 0},
			Info:    &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
				Block: &netstats.Block{
					Number:       50,
					Hash:         "0x0032",
					ParentHash:   "0x0033",
					Arrived:      946684850000,
					Received:     946684850000,
					Transactions: []*netstats.Transaction{},
				},
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.AddBlock(ctx, "X", &netstats.Block{}); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ErrBlockRequired", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		} else if err := db.AddBlock(ctx, "X", nil); err != netstats.ErrBlockRequired {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_AddBlocks(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		notify := db.BestBlockNumberNotify()

		if err := db.AddBlocks(ctx, "X", []*netstats.Block{
			{
				Number:          1,
				TotalDifficulty: 10,
				Hash:            "0x0001",
				ParentHash:      "0x0000",
				Transactions:    []*netstats.Transaction{{Hash: "0x000A"}, {Hash: "0x000B"}},
			},
			{
				Number:          2,
				TotalDifficulty: 20,
				Hash:            "0x0002",
				ParentHash:      "0x0001",
				Transactions:    []*netstats.Transaction{{Hash: "0x000C"}, {Hash: "0x000D"}},
			},
			{
				Number:          3,
				TotalDifficulty: 30,
				Hash:            "0x0003",
				ParentHash:      "0x0002",
				Transactions:    []*netstats.Transaction{{Hash: "0x000E"}, {Hash: "0x000F"}},
			},
		}); err != nil {
			t.Fatal(err)
		}

		// Ensure best block number notification occurs.
		select {
		case <-notify:
		default:
			t.Fatal("expected best block number notification")
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:   "X",
			Geo:  &netstats.Geo{LL: []float64{0, 0}},
			Info: &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: true,
				Uptime: 100,
			},
			Uptime: &netstats.Uptime{
				LastStatus: true,
				LastUpdate: 946684800000,
				Started:    946684800000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.AddBlocks(ctx, "X", []*netstats.Block{}); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_UpdateLatency(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		if err := db.UpdateLatency(ctx, "X", 1000); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if node.Stats.Latency != 1000 {
			t.Fatalf("unexpected latency: %v", node.Stats.Latency)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.UpdateLatency(ctx, "X", 1000); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDB_SetInactive(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(time.Second)
		if err := db.SetInactive(ctx, "X"); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:   "X",
			Geo:  &netstats.Geo{LL: []float64{0, 0}},
			Info: &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: false,
				Uptime: 100,
			},
			Uptime: &netstats.Uptime{
				LastStatus: false,
				LastUpdate: 946684801000,
				Started:    946684800000,
				Up:         1000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("Toggle", func(t *testing.T) {
		db := NewDB()
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(time.Second)
		if err := db.SetInactive(ctx, "X"); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(10 * time.Second)
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(20 * time.Second)
		if err := db.CreateNodeIfNotExists(ctx, &netstats.Node{ID: "X"}); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(2 * time.Second)
		if err := db.SetInactive(ctx, "X"); err != nil {
			t.Fatal(err)
		}

		db.Now = db.Now.Add(3 * time.Second)
		if err := db.SetInactive(ctx, "X"); err != nil {
			t.Fatal(err)
		}

		if node, err := db.FindNodeByID(ctx, "X"); err != nil {
			t.Fatal(err)
		} else if diff := cmp.Diff(node, &netstats.Node{
			ID:   "X",
			Geo:  &netstats.Geo{LL: []float64{0, 0}},
			Info: &netstats.NodeInfo{},
			Stats: &netstats.Stats{
				Active: false,
				Uptime: 63,
			},
			Uptime: &netstats.Uptime{
				LastStatus: false,
				LastUpdate: 946684836000,
				Started:    946684800000,
				Up:         23000,
				Down:       13000,
			},
		}); diff != "" {
			t.Fatal(diff)
		}
	})

	t.Run("ErrNodeNotFound", func(t *testing.T) {
		db := NewDB()
		if err := db.SetInactive(ctx, "X"); err != netstats.ErrNodeNotFound {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// DB is a test wrapper for netstats.DB that allows mocking of current time.
type DB struct {
	*netstats.DB
	Now time.Time
}

func NewDB() *DB {
	db := &DB{
		DB:  netstats.NewDB("test"),
		Now: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
	db.DB.Now = func() int64 { return int64(db.Now.UnixNano() / int64(time.Millisecond)) }
	return db
}
