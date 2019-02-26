package netstats

import (
	"context"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"
)

type DB struct {
	mu    sync.RWMutex
	nodes map[string]*Node // items -- was array

	blocks       map[string]*Block         // blocks by hash
	propagations map[string][]*Propagation // propagations by hash

	bestBlockNumberNotify chan struct{} // notify clients of new best block#

	chart       *Chart        // cached chart data
	chartNotify chan struct{} // notify clients of new chart

	// Set of geolocations for trusted IPs. Must be set before using DB.
	GeoByIP map[string]*Geo

	// Geolocation service used for IPs not in GeoByIP.
	GeoService GeoService

	// Now returns the current time, in milliseconds.
	Now func() int64
}

func NewDB() *DB {
	db := &DB{
		nodes:        make(map[string]*Node),
		blocks:       make(map[string]*Block),
		propagations: make(map[string][]*Propagation),

		bestBlockNumberNotify: make(chan struct{}),

		chartNotify: make(chan struct{}),

		GeoByIP:    make(map[string]*Geo),
		GeoService: &NopGeoService{},

		Now: func() int64 { return int64(time.Now().UnixNano() / int64(time.Millisecond)) },
	}
	db.chart = db.bestBlockchain().Chart()
	return db
}

// BestBlockNumberNotify returns a channel that closes when a new best block number increases.
func (db *DB) BestBlockNumberNotify() <-chan struct{} {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.bestBlockNumberNotify
}

// notifyBestBlockNumber closes the current notify channel and creates a new one.
func (db *DB) notifyBestBlockNumber() {
	close(db.bestBlockNumberNotify)
	db.bestBlockNumberNotify = make(chan struct{})
}

// Chart returns the last cached chart data.
func (db *DB) Chart() *Chart {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.chart
}

// ChartNotify returns a channel that closes when new charts are available.
func (db *DB) ChartNotify() <-chan struct{} {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.chartNotify
}

// notifyChart closes the current notify channel and creates a new one.
func (db *DB) notifyChart() {
	db.chart = db.bestBlockchain().Chart()
	close(db.chartNotify)
	db.chartNotify = make(chan struct{})
}

// FindNodeByID returns a node by node ID.
// Returns ErrNodeNotFound if node does not exist.
func (db *DB) FindNodeByID(ctx context.Context, id string) (*Node, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	node := db.nodes[id]
	if node == nil {
		return nil, ErrNodeNotFound
	}
	return node.Clone(), nil
}

// Nodes returns a list of all nodes, sorted by id.
func (db *DB) Nodes(ctx context.Context) ([]*Node, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	a := make([]*Node, 0, len(db.nodes))
	for _, node := range db.nodes {
		a = append(a, node.Clone())
	}
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	return a, nil
}

// Blocks returns a list of all blocks.
func (db *DB) Blocks(ctx context.Context) ([]*Block, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	a := make([]*Block, 0, len(db.blocks))
	for _, block := range db.blocks {
		a = append(a, block.Clone())
	}
	sort.Slice(a, func(i, j int) bool { return CompareBlocks(a[i], a[j]) == -1 })
	return a, nil
}

// CreateNodeIfNotExists adds the node to the database if it is not already registered.
func (db *DB) CreateNodeIfNotExists(ctx context.Context, node *Node) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Update geolocation.
	node = node.Clone()
	if node.Geo == nil {
		node.Geo = &Geo{LL: []float64{0, 0}}
	}
	if node.Info != nil && node.Info.IP != "" {
		if geo := db.GeoByIP[node.Info.IP]; geo != nil {
			node.Trusted = true
			node.Geo = geo.Clone()
			log.Printf("Node %q is a trusted node: %#v", node.Info.IP, node.Geo)
		} else if geo, err := db.GeoService.GeoByIP(context.Background(), node.Info.IP); err != nil {
			log.Printf("Cannot find geolocation by ip: %s", node.Info.IP)
		} else if geo != nil {
			node.Geo = geo.Clone()
			log.Printf("Node %q is an unknown node: %#v", node.Info.IP, node.Geo)
		}
	}

	// If node already exists, simply mark it as active.
	if n := db.nodes[node.ID]; n != nil {
		n.Info = node.Info
		n.setState(true, db.Now())
		return nil
	}

	// Otherwise initialize.
	if node.Info == nil {
		node.Info = &NodeInfo{}
	}
	if node.Stats == nil {
		node.Stats = &Stats{}
	}
	if node.Uptime == nil {
		node.Uptime = &Uptime{}
	}
	db.nodes[node.ID] = node

	// Mark node as active.
	node.setState(true, db.Now())

	return nil
}

// UpdatePending updates the pending stat for a given node.
func (db *DB) UpdatePending(ctx context.Context, nodeID string, pending int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[nodeID]
	if node == nil {
		return ErrNodeNotFound
	}
	node.Stats.Pending = pending

	return nil
}

// UpdateStats updates basic stats for a given node.
func (db *DB) UpdateStats(ctx context.Context, nodeID string, stats Stats) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[nodeID]
	if node == nil {
		return ErrNodeNotFound
	}

	node.Stats.Active = stats.Active
	node.Stats.Mining = stats.Mining
	node.Stats.Syncing = stats.Syncing
	node.Stats.Hashrate = stats.Hashrate
	node.Stats.Peers = stats.Peers
	node.Stats.GasPrice = stats.GasPrice
	node.Stats.Uptime = stats.Uptime

	return nil
}

// AddBlock adds a block to the database on behalf of a node. Updates the
// propagation stats for the block & node based on the current time.
func (db *DB) AddBlock(ctx context.Context, nodeID string, block *Block) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[nodeID]
	if node == nil {
		return ErrNodeNotFound
	} else if block == nil {
		return ErrBlockRequired
	}

	now := db.Now()
	block.Arrived, block.Received = now, now
	block.Trusted = node.Trusted

	bestBlockNumber := db.bestBlockNumber()

	db.addBlock(ctx, block)
	db.addBlockPropagation(ctx, block.Hash, nodeID, now)
	db.trim()

	// If there is a higher block number then notify clients.
	if db.bestBlockNumber() > bestBlockNumber {
		db.notifyBestBlockNumber()
	}

	if bc := db.bestBlockchain(); bc != nil && bc.Len() > 0 {
		history := bc.NodePropagation(nodeID)
		if node.Stats.Block == nil || block.Number != node.Stats.Block.Number || block.Hash != node.Stats.Block.Hash {
			node.Stats.Block = block.Clone()
			node.Stats.Block.Propagation = history[len(history)-1]
		}
		node.setHistory(history)
	}

	db.notifyChart()

	return nil
}

// AddBlocks adds one or more blocks on behalf of a node.
func (db *DB) AddBlocks(ctx context.Context, nodeID string, blocks []*Block) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[nodeID]
	if node == nil {
		return ErrNodeNotFound
	}

	bestBlockNumber := db.bestBlockNumber()

	now := db.Now()
	for _, block := range blocks {
		block.Trusted = node.Trusted
		block.Arrived, block.Received = now, now

		db.addBlock(ctx, block)
		db.addBlockPropagation(ctx, block.Hash, nodeID, now)
		db.trim()
	}

	// If there is a higher block number then notify clients.
	if db.bestBlockNumber() > bestBlockNumber {
		db.notifyBestBlockNumber()
	}

	db.notifyChart()

	return nil
}

// UpdateLatency sets the latency stat for a given node.
func (db *DB) UpdateLatency(ctx context.Context, id string, latency int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[id]
	if node == nil {
		return ErrNodeNotFound
	}
	node.Stats.Latency = latency
	return nil
}

// SetInactive marks a given node as inactive.
func (db *DB) SetInactive(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	node := db.nodes[id]
	if node == nil {
		return ErrNodeNotFound
	}
	node.setState(false, db.Now())
	return nil
}

// BestBlockNumber returns the block number of the best block.
func (db *DB) BestBlockNumber(ctx context.Context) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.bestBlockNumber()
}

// MaxBlockNumber returns the block number of the highest block.
func (db *DB) MaxBlockNumber(ctx context.Context) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.maxBlockNumber()
}

// MinBlockNumber returns the block number of the lowest block.
func (db *DB) MinBlockNumber(ctx context.Context) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.minBlockNumber()
}

// addBlock adds the block to the database if it hasn't been added yet.
func (db *DB) addBlock(ctx context.Context, block *Block) {
	if _, ok := db.blocks[block.Hash]; ok {
		return
	}

	// Clone block and initialize fields.
	block = block.Clone()
	block.Propagation = 0
	block.Time = 0

	// Calculate time based on parent block arrival time.
	if parent := db.blocks[block.ParentHash]; parent != nil {
		block.Time = block.Arrived - parent.Arrived
	}

	db.blocks[block.Hash] = block
}

// addBlockPropagation adds node propagation information for a given block.
func (db *DB) addBlockPropagation(ctx context.Context, hash, nodeID string, received int64) {
	// Exit if node propagation already exists.
	propagations := db.propagations[hash]
	for i := range propagations {
		if propagations[i].NodeID == nodeID {
			return
		}
	}

	// Determine first received time for this block.
	initial := received
	if len(propagations) > 0 {
		initial = propagations[0].Received
	}

	// Save propagation time for node & sort by received time.
	propagations = append(propagations, &Propagation{
		NodeID:      nodeID,
		Received:    received,
		Propagation: received - initial,
	})
	sort.Slice(propagations, func(i, j int) bool { return propagations[i].Received < propagations[j].Received })

	// Update propagation list.
	db.propagations[hash] = propagations
}

func (db *DB) bestBlockchain() *Blockchain {
	bc := NewBlockchain()
	for block := db.bestBlock(); block != nil && bc.Len() <= MaxHistory; block = db.blocks[block.ParentHash] {
		bc.blocks = append([]*Block{block}, bc.blocks...)
		bc.propagations = append([][]*Propagation{db.propagations[block.Hash]}, bc.propagations...)
	}
	bc.Trim(MaxHistory)
	return bc
}

func (db *DB) bestBlock() *Block {
	var best *Block
	for _, block := range db.blocks {
		if best == nil {
			best = block
			continue
		}
		if cmp := CompareBlocks(best, block); cmp == -1 || (cmp == 0 && rand.Intn(2) == 0) {
			best = block
			continue
		}
	}
	return best
}

func (db *DB) bestBlockNumber() int {
	block := db.bestBlock()
	if block == nil {
		return 0
	}
	return block.Number
}

func (db *DB) maxBlockNumber() int {
	var max int
	for _, block := range db.blocks {
		if block.Number > max {
			max = block.Number
		}
	}
	return max
}

func (db *DB) minBlockNumber() int {
	var min int
	for _, block := range db.blocks {
		if min == 0 || block.Number < min {
			min = block.Number
		}
	}
	return min
}

// trim removes all blocks below a given threshold from the highest block.
func (db *DB) trim() {
	minBlockNumber := db.bestBlockNumber() - MaxHistory
	for _, block := range db.blocks {
		if block.Number < minBlockNumber {
			delete(db.blocks, block.Hash)
			delete(db.propagations, block.Hash)
		}
	}
}

// RemoveOldNodes removes nodes that aren't active and haven't been updated recently.
func (db *DB) RemoveOldNodes() {
	db.mu.Lock()
	defer db.mu.Unlock()

	for key, node := range db.nodes {
		if !node.Uptime.LastStatus && db.Now()-node.Uptime.LastUpdate > MaxInactiveTime {
			delete(db.nodes, key)
		}
	}
}
