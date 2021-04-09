package netstats

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	ErrNodeNotFound  = errors.New("node not found")
	ErrBlockRequired = errors.New("block required")
)

const (
	MaxHistory         = 40
	MaxPeerPropagation = 40
	MaxInactiveTime    = 4 * 60 * 60 * 1000

	// Histogram constants
	MaxBins             = 40
	MinPropagationRange = 0
	MaxPropagationRange = 10000
)

type Block struct {
	Number           int            `json:"number"`
	Hash             string         `json:"hash"`
	ParentHash       string         `json:"parentHash"`
	Timestamp        int            `json:"timestamp"`
	Miner            string         `json:"miner"`
	GasUsed          int            `json:"gasUsed"`
	GasLimit         int            `json:"gasLimit"`
	Difficulty       int            `json:"difficulty,string"`
	TotalDifficulty  int            `json:"totalDifficulty,string"`
	Transactions     []*Transaction `json:"transactions"`
	TransactionCount int            `json:"transactionCount"`
	TransactionsRoot string         `json:"transactionsRoot"`
	StateRoot        string         `json:"stateRoot"`
	Uncles           []interface{}  `json:"uncles"`
	Trusted          bool           `json:"trusted"`
	Arrived          int64          `json:"arrived"`
	Received         int64          `json:"received"`
	Propagation      int64          `json:"propagation"`
	Time             int64          `json:"time"` // Time since last block (ms)
	Fork             int            `json:"fork"`
}

func (b *Block) Clone() *Block {
	other := *b
	other.Transactions = make([]*Transaction, len(b.Transactions))
	for i := range other.Transactions {
		other.Transactions[i] = b.Transactions[i].Clone()
	}
	return &other
}

// CompareBlocks compares two blocks by difficulty, number, & gas used.
// Returns 0 if a==b, -1 if a < b, and +1 if a > b.
func CompareBlocks(a, b *Block) int {
	if a.TotalDifficulty < b.TotalDifficulty {
		return -1
	} else if a.TotalDifficulty > b.TotalDifficulty {
		return 1
	}

	if a.Number < b.Number {
		return -1
	} else if a.Number > b.Number {
		return 1
	}

	if a.GasUsed < b.GasUsed {
		return -1
	} else if a.GasUsed > b.GasUsed {
		return 1
	}
	return 0
}

type Transaction struct {
	Hash string `json:"hash"`
}

func (t *Transaction) Clone() *Transaction {
	other := *t
	return &other
}

type Stats struct {
	Active         bool   `json:"active"`
	Mining         bool   `json:"mining"`
	Syncing        bool   `json:"syncing"`
	Hashrate       int    `json:"hashrate"`
	Peers          int    `json:"peers"`
	GasPrice       int    `json:"gasPrice"`
	Uptime         int64  `json:"uptime"`
	Pending        int    `json:"pending"`
	Latency        int64  `json:"latency,string"`
	PropagationAvg int64  `json:"propagationAvg"`
	Block          *Block `json:"block,omitempty"`
}

// Clone returns a deep copy of s.
func (s *Stats) Clone() *Stats {
	other := *s
	if s.Block != nil {
		other.Block = s.Block.Clone()
	}
	return &other
}

type Node struct {
	ID             string    `json:"id,omitempty"`
	Geo            *Geo      `json:"geo,omitempty"`
	Block          *Block    `json:"block,omitempty"`
	History        []int64   `json:"history,omitempty"`
	Info           *NodeInfo `json:"info,omitempty"`
	Stats          *Stats    `json:"stats,omitempty"`
	Trusted        bool      `json:"trusted,omitempty"`
	PropagationAvg int64     `json:"propagationAvg,omitempty"`
	Uptime         *Uptime   `json:"uptime,omitempty"`
}

func NewNode(timestamp int64) *Node {
	node := &Node{
		Info: &NodeInfo{},
		Geo:  &Geo{},
		Stats: &Stats{
			Block: &Block{
				Hash:         "0x0000000000000000000000000000000000000000000000000000000000000000",
				Transactions: make([]*Transaction, 0),
				Uncles:       make([]interface{}, 0),
			},
			Uptime: 100,
		},
		History: make([]int64, MaxHistory),
		Uptime:  &Uptime{},
	}

	for i := range node.History {
		node.History[i] = -1
	}

	if node.ID == "" && node.Uptime.Started == 0 {
		node.setState(true, timestamp)
	}

	return node
}

func (n *Node) setState(active bool, timestamp int64) {
	// Add time since last update as either uptime or downtime depending on status.
	if n.Uptime.Started != 0 {
		if n.Uptime.LastStatus == active {
			if active {
				n.Uptime.Up += timestamp - n.Uptime.LastUpdate
			} else {
				n.Uptime.Down += timestamp - n.Uptime.LastUpdate
			}
		} else {
			if active {
				n.Uptime.Down += timestamp - n.Uptime.LastUpdate
			} else {
				n.Uptime.Up += timestamp - n.Uptime.LastUpdate
			}
		}
	} else {
		n.Uptime.Started = timestamp
	}

	n.Stats.Active = active
	n.Uptime.LastStatus = active
	n.Uptime.LastUpdate = timestamp

	// Calculate uptime.
	if n.Uptime.LastUpdate == n.Uptime.Started {
		n.Stats.Uptime = 100
	} else {
		n.Stats.Uptime = (n.Uptime.Up * 100) / (n.Uptime.LastUpdate - n.Uptime.Started) // integer percentage
	}
}

func (n *Node) getInfo() *Node {
	return &Node{
		ID:   n.ID,
		Info: n.Info,
		Stats: &Stats{
			Active:         n.Stats.Active,
			Mining:         n.Stats.Mining,
			Syncing:        n.Stats.Syncing,
			Hashrate:       n.Stats.Hashrate,
			Peers:          n.Stats.Peers,
			GasPrice:       n.Stats.GasPrice,
			Block:          n.Stats.Block,
			PropagationAvg: n.Stats.PropagationAvg,
			Uptime:         n.Stats.Uptime,
			Latency:        n.Stats.Latency,
			Pending:        n.Stats.Pending,
		},
		History: n.History,
		Geo:     n.Geo,
	}
}

// setHistory sets the history and computes the new propagation average for the node.
func (n *Node) setHistory(history []int64) {
	n.History = int64SliceClone(history)

	var sum, count int64
	for _, v := range history {
		if v >= 0 {
			sum += v
			count++
		}
	}

	if count > 0 {
		n.Stats.PropagationAvg = int64(math.Round(float64(sum) / float64(count)))
	} else {
		n.Stats.PropagationAvg = 0
	}
}

func (n *Node) getStats() *Node {
	return &Node{
		ID:      n.ID,
		History: n.History,

		Stats: &Stats{
			Active:         n.Stats.Active,
			Mining:         n.Stats.Mining,
			Syncing:        n.Stats.Syncing,
			Hashrate:       n.Stats.Hashrate,
			Peers:          n.Stats.Peers,
			GasPrice:       n.Stats.GasPrice,
			Block:          n.Stats.Block,
			PropagationAvg: n.Stats.PropagationAvg,
			Uptime:         n.Stats.Uptime,
			Pending:        n.Stats.Pending,
			Latency:        n.Stats.Latency,
		},
	}
}

func (n *Node) getBlockStats() *Node {
	return &Node{
		ID:             n.ID,
		Block:          n.Stats.Block,
		PropagationAvg: n.Stats.PropagationAvg,
		History:        n.History,
	}
}

func (n *Node) BlockNumber() int {
	if n.Stats == nil || n.Stats.Block == nil {
		return 0
	}
	return n.Stats.Block.Number
}

func (n *Node) canUpdate() bool {
	if n.Trusted {
		return true
	}
	return n.Info.CanUpdateHistory || (!n.Stats.Syncing && n.Stats.Peers > 0)
}

func (n *Node) isInactiveAndOld() bool {
	return !n.Uptime.LastStatus && n.Uptime.LastUpdate != 0 && n.Uptime.LastUpdate > MaxInactiveTime
}

// Clone returns a deep copy of n.
func (n *Node) Clone() *Node {
	other := *n
	if n.Geo != nil {
		other.Geo = n.Geo.Clone()
	}
	if n.Block != nil {
		other.Block = n.Block.Clone()
	}
	if n.Info != nil {
		other.Info = n.Info.Clone()
	}
	if n.Stats != nil {
		other.Stats = n.Stats.Clone()
	}
	if n.Uptime != nil {
		other.Uptime = n.Uptime.Clone()
	}
	return &other
}

// ErrIPNotFound is returned by GeoService.GeoByIP if an IP address is not found.
var ErrIPNotFound = errors.New("ip address not found")

// GeoService represents a service for computing geolocation by IP address.
type GeoService interface {
	GeoByIP(ctx context.Context, ip string) (*Geo, error)
}

// NopGeoService is a GeoService that does nothing.
type NopGeoService struct{}

// GeoByIP always returns ErrIPNotFound.
func (s *NopGeoService) GeoByIP(ctx context.Context, ip string) (*Geo, error) {
	return nil, ErrIPNotFound
}

type Trusted struct {
	ID string `json:"id"`
	Geo
}

type TrustedByIP map[string]*Trusted

func (tp TrustedByIP) MarshalLogObject(oe zapcore.ObjectEncoder) error {
	for ip, t := range tp {
		if err := oe.AddObject(ip, t); err != nil {
			return err
		}
	}
	return nil
}

type Geo struct {
	City    string         `json:"city"`
	Country string         `json:"country"`
	LL      [2]json.Number `json:"ll"`
	Metro   int            `json:"metro"`
	Range   []int          `json:"range"`
	Region  string         `json:"region"`
	Zip     int            `json:"zip"`
}

func (g *Geo) MarshalLogObject(oe zapcore.ObjectEncoder) error {
	oe.AddString("city", g.City)
	oe.AddString("country", g.Country)
	zap.String("lat", g.LL[0].String()).AddTo(oe)
	zap.String("long", g.LL[1].String()).AddTo(oe)
	zap.Int("metro", g.Metro)
	zap.Ints("range", g.Range)
	zap.String("region", g.Region)
	zap.Int("zip", g.Zip)
	return nil
}

// Clone returns a deep copy of g.
func (g *Geo) Clone() *Geo {
	other := *g
	other.LL = g.LL
	if g.Range != nil {
		other.Range = make([]int, len(g.Range))
		copy(other.Range, g.Range)
	}
	return &other
}

// This data is reported by the node and forwarded to clients,
// but the Name may be modified for trusted nodes.
type NodeInfo struct {
	API              string `json:"api,omitempty"`
	CanUpdateHistory bool   `json:"canUpdateHistory,omitempty"`
	Client           string `json:"client,omitempty"`
	IP               string `json:"ip,omitempty"`
	Name             string `json:"name,omitempty"`
	Net              string `json:"net,omitempty"`
	Node             string `json:"node,omitempty"`
	OS               string `json:"os,omitempty"`
	OS_V             string `json:"os_v,omitempty"`
	Port             int    `json:"port,omitempty"`
	Protocol         string `json:"protocol,omitempty"`
}

// Clone returns a deep copy of ni.
func (ni *NodeInfo) Clone() *NodeInfo {
	other := *ni
	return &other
}

type Uptime struct {
	Down       int64 `json:"down"`
	LastStatus bool  `json:"lastStatus"`
	LastUpdate int64 `json:"lastUpdate"`
	Started    int64 `json:"started"`
	Up         int64 `json:"up"`
}

// Clone returns a deep copy of ni.
func (u *Uptime) Clone() *Uptime {
	other := *u
	return &other
}

type Blockchain struct {
	networkName  string
	blocks       []*Block
	propagations [][]*Propagation
}

func NewBlockchain() *Blockchain {
	return &Blockchain{}
}

func (bc *Blockchain) Len() int { return len(bc.blocks) }

func (bc *Blockchain) MinBlockNumber() int {
	if len(bc.blocks) == 0 {
		return 0
	}
	return bc.blocks[0].Number
}

func (bc *Blockchain) MaxBlockNumber() int {
	if len(bc.blocks) == 0 {
		return 0
	}
	return bc.blocks[len(bc.blocks)-1].Number
}

func (bc *Blockchain) BlockNumberIndex(number int) int {
	return sort.Search(len(bc.blocks), func(i int) bool { return bc.blocks[i].Number >= number })
}

// HasHash returns true if a block in the chain exists with the given hash.
func (bc *Blockchain) HasHash(hash string) bool {
	for _, block := range bc.blocks {
		if block.Hash == hash {
			return true
		}
	}
	return false
}

// Trim ensures that the chain length is not greater than n blocks.
func (bc *Blockchain) Trim(n int) {
	if x := bc.Len() - n; x > 0 {
		bc.blocks = bc.blocks[x:]
		bc.propagations = bc.propagations[x:]
	}
}

func (bc *Blockchain) NodePropagation(id string) []int64 {
	bestBlock := bc.MaxBlockNumber()
	// lastBlockTime := time.Now()

	history := make([]int64, MaxPeerPropagation)
	for i := range history {
		history[i] = -1
	}

	for i, propagations := range bc.propagations {
		propagation := PropagationByNodeID(propagations, id)
		if propagation == nil {
			continue
		}
		if i := MaxPeerPropagation - 1 - bestBlock + bc.blocks[i].Number; i > 0 && i < len(history) {
			history[i] = propagation.Propagation
		}
	}

	return history
}

func (bc *Blockchain) BlockPropagation() *BlockPropagationStats {
	// Build fixed width histogram.
	histogram := make([]*BlockPropagationStat, MaxBins)
	dx := MaxPropagationRange / MaxBins
	for i := range histogram {
		histogram[i] = &BlockPropagationStat{
			X:  i * dx,
			Dx: dx,
		}
	}

	// Add propagations to the appropriate histogram buckets.
	var propagationN int
	for i := range bc.propagations {
		for j := range bc.propagations[i] {
			if v := int(bc.propagations[i][j].Propagation); v >= 0 {
				if v >= MaxPropagationRange {
					v = MaxPropagationRange - 1
				}
				histogram[v/dx].Frequency++
				propagationN++
			}
		}
	}

	// Calculate cumulative values.
	var cumulative int
	for i := range histogram {
		cumulative += histogram[i].Frequency
		histogram[i].Cumulative = cumulative

		if propagationN > 0 {
			histogram[i].Cumpercent = float64(cumulative) / float64(propagationN)
			histogram[i].Y = float64(histogram[i].Frequency) / float64(propagationN)
		}
	}

	return &BlockPropagationStats{
		Avg:       bc.AvgBlockPropagation(),
		Histogram: histogram,
	}
}

func (bc *Blockchain) AvgBlockPropagation() int64 {
	var propagationN, propagationSum int64
	for _, propagations := range bc.propagations {
		for _, p := range propagations {
			if p.Propagation < MinPropagationRange {
				continue
			}

			v := p.Propagation
			if v > MaxPropagationRange {
				v = MaxPropagationRange
			}
			propagationSum += v
			propagationN++
		}
	}

	if propagationN == 0 {
		return 0
	}
	return propagationSum / propagationN
}

func (bc *Blockchain) Chart() *Chart {
	chart := NewChart()
	chart.NetworkName = bc.networkName
	if len(bc.blocks) == 0 {
		return chart
	}
	bc.Trim(MaxHistory)

	maxBlockNumber, minBlockNumber := bc.MaxBlockNumber(), bc.MinBlockNumber()
	if minBlockNumber+MaxHistory > maxBlockNumber {
		minBlockNumber = maxBlockNumber - MaxHistory
	}

	var sumTime int64
	var transactionCount int
	for i := 0; i < bc.Len() && i < MaxHistory; i++ {
		block := bc.blocks[i]

		var transactionRate int
		if len(block.Transactions) > 0 && block.Time > 0 {
			transactionRate = int(float64(len(block.Transactions)) / (float64(block.Time) / 1000.0))
		}

		pos := MaxHistory - bc.Len() + i
		chart.Height[pos] = block.Number
		chart.BlockTime[pos] = float64(block.Time) / 1000.0
		chart.Difficulty[pos] = strconv.Itoa(block.Difficulty)
		chart.Uncles[pos] = len(block.Uncles)
		chart.Transactions[pos] = len(block.Transactions)
		chart.TransactionRate[pos] = strconv.Itoa(transactionRate)
		chart.GasSpending[pos] = block.GasUsed
		chart.GasLimit[pos] = block.GasLimit

		sumTime += block.Time
		transactionCount += len(block.Transactions)
	}

	chart.AvgBlockTime = bc.AvgBlockTime()
	chart.Miners = bc.MinersCount()
	chart.Propagation = bc.BlockPropagation()
	if len(bc.blocks) > 0 && sumTime >= 1000 {
		avgTransactionRate := decimal.NewFromInt(int64(transactionCount)).
			Div(decimal.New(sumTime, -3))
		if avgTransactionRate.LessThan(decimal.NewFromInt(1)) {
			chart.AvgTransactionRate = json.Number(avgTransactionRate.StringFixed(2))
		} else {
			chart.AvgTransactionRate = json.Number(avgTransactionRate.StringFixed(0))
		}
		chart.AvgHashRate = (bc.blocks[len(bc.blocks)-1].Difficulty / (int(sumTime) / len(bc.blocks)) / 1000)
	}

	return chart
}

// AvgBlockTime returns the average block time in seconds.
func (bc *Blockchain) AvgBlockTime() float64 {
	if len(bc.blocks) == 0 {
		return 0
	}

	var sum, cnt int64
	for _, b := range bc.blocks {
		if b.Time == 0 {
			continue
		}
		sum += b.Time
		cnt++
	}
	if sum == 0 || cnt == 0 {
		return 0
	}
	return float64(sum) / float64(cnt) / 1000
}

func (bc *Blockchain) MinersCount() []*ChartMiner {
	m := make(map[string]int)
	for _, block := range bc.blocks {
		m[block.Miner]++
	}

	a := make([]*ChartMiner, 0, len(m))
	for miner, count := range m {
		a = append(a, &ChartMiner{Miner: miner, Blocks: count})
	}
	sort.Slice(a, func(i, j int) bool { return a[i].Blocks > a[j].Blocks })

	if len(a) > 2 {
		a = a[2:]
	}
	return a
}

type Chart struct {
	NetworkName        string        `json:"networkName"`
	Height             []int         `json:"height"`
	BlockTime          []float64     `json:"blocktime"`
	AvgBlockTime       float64       `json:"avgBlocktime"` // Average block time (s)
	Difficulty         []string      `json:"difficulty"`
	Uncles             []int         `json:"uncles"`
	Transactions       []int         `json:"transactions"`
	TransactionRate    []string      `json:"transactionrate"`
	GasSpending        []int         `json:"gasSpending"`
	GasLimit           []int         `json:"gasLimit"`
	Miners             []*ChartMiner `json:"miners"`
	UncleCount         []int         `json:"uncleCount"`
	AvgHashRate        int           `json:"avgHashrate"`
	AvgTransactionRate json.Number   `json:"avgTransactionRate"`

	Propagation *BlockPropagationStats `json:"propagation"`
}

func NewChart() *Chart {
	return &Chart{
		Height:          make([]int, MaxHistory),
		BlockTime:       make([]float64, MaxHistory),
		Difficulty:      make([]string, MaxHistory),
		Uncles:          make([]int, MaxHistory),
		Transactions:    make([]int, MaxHistory),
		TransactionRate: make([]string, MaxHistory),
		GasSpending:     make([]int, MaxHistory),
		GasLimit:        make([]int, MaxHistory),
		UncleCount:      make([]int, MaxHistory),
		Propagation:     &BlockPropagationStats{},
	}
}

type ChartMiner struct {
	Miner  string `json:"miner"`
	Name   bool   `json:"name"`
	Blocks int    `json:"blocks"`
}

type Propagation struct {
	NodeID      string
	Received    int64
	Propagation int64
}

func PropagationByNodeID(a []*Propagation, nodeID string) *Propagation {
	for i := range a {
		if a[i].NodeID == nodeID {
			return a[i]
		}
	}
	return nil
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func int64SliceClone(a []int64) []int64 {
	other := make([]int64, len(a))
	copy(other, a)
	return other
}

type BlockPropagationStats struct {
	Histogram []*BlockPropagationStat `json:"histogram"`
	Avg       int64                   `json:"avg"`
}

type BlockPropagationStat struct {
	X          int     `json:"x"`
	Dx         int     `json:"dx"`
	Y          float64 `json:"y"`
	Frequency  int     `json:"frequency"`
	Cumulative int     `json:"cumulative"`
	Cumpercent float64 `json:"cumpercent"`
}

func assert(condition bool, msg string) {
	if !condition {
		panic("assertion failed: " + msg)
	}
}
