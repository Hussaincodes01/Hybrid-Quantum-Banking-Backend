package fraud

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"sync"
	"time"
)

// FraudGraph implements a Graph Neural Network-style mule account detection.
// Per spec §3A.3 Layer 2: "A Graph Neural Network (GNN) models the network of
// payment relationships across the (anonymised) user base."
//
// This is a simplified graph-based risk propagation system that:
// 1. Builds a payment graph (nodes = recipients, edges = transactions)
// 2. Marks known fraud nodes
// 3. Propagates risk through the graph using N-hop scoring
// 4. Returns a risk score [0,1] for any recipient based on graph proximity to fraud

const (
	maxHops         = 3
	riskDecayFactor = 0.6
	fraudNodeRisk   = 1.0
	maxGraphSize    = 10000
)

type GraphNode struct {
	ID         string
	IsFraud    bool
	IsMule     bool
	RiskScore  float64
	Edges      map[string]*GraphEdge
	LastUpdate time.Time
}

type GraphEdge struct {
	To        string
	Weight    float64
	TxCount   int
	TotalAmt  int64
	FirstTxAt time.Time
	LastTxAt  time.Time
}

// L-8 structuring parameters: the edge weight must not depend on transaction
// COUNT alone, or an attacker splits one large transfer into many small ones to
// keep the count (hence the weight) low. We also factor cumulative amount and
// velocity (transfers per day) so smurfing patterns surface.
const (
	structuringBenchmarkPaise = 100000000 // ₹10,00,000 cumulative → full amount signal
	structuringCountFull      = 10.0      // ~10 transfers → full count signal
	velocityPerDayFull        = 10.0      // ~10 transfers/day → full velocity signal
)

// structuringWeight combines count, cumulative amount, and velocity into [0,1].
func structuringWeight(e *GraphEdge) float64 {
	countSignal := float64(e.TxCount) / structuringCountFull
	amtSignal := float64(e.TotalAmt) / float64(structuringBenchmarkPaise)
	velSignal := 0.0
	// Velocity is a RATE, so it needs at least two transfers to be meaningful. With
	// a single transaction FirstTxAt == LastTxAt, and flooring the window would make
	// one transfer read as ~24/day and wrongly saturate the weight. Only measure the
	// rate once there are 2+ transfers spanning a real interval.
	if e.TxCount >= 2 && !e.FirstTxAt.IsZero() {
		days := e.LastTxAt.Sub(e.FirstTxAt).Hours() / 24.0
		if days < 1.0/24.0 {
			days = 1.0 / 24.0 // floor at 1h so a rapid burst reads as high velocity
		}
		velSignal = (float64(e.TxCount) / days) / velocityPerDayFull
	}
	return math.Min(math.Max(countSignal, math.Max(amtSignal, velSignal)), 1.0)
}

type FraudGraph struct {
	mu    sync.RWMutex
	nodes map[string]*GraphNode
	// inEdges is the reverse adjacency index: inEdges[target][sender] is the SAME
	// *GraphEdge as nodes[sender].Edges[target]. It lets computeNHopRisk find the
	// senders into a node in O(in-degree) instead of scanning every node.
	inEdges map[string]map[string]*GraphEdge
	// gen is bumped on every graph mutation. Cached scores carry the gen they were
	// computed under; a mismatch means the cache entry is logically expired, so a
	// single increment invalidates the whole cache in O(1) without clearing it.
	gen uint64
	// cache memoises the TOP-LEVEL GetRiskScore result per node. Do NOT memoise the
	// inner recursion by (node, hops): that result is path-dependent on the visited
	// set and caching it returns wrong risk when a node is reached via another path.
	cache map[string]cachedRisk
}

type cachedRisk struct {
	risk float64
	gen  uint64
}

func NewFraudGraph() *FraudGraph {
	fg := &FraudGraph{
		nodes:   make(map[string]*GraphNode),
		inEdges: make(map[string]map[string]*GraphEdge),
		cache:   make(map[string]cachedRisk),
	}
	fg.seedKnownFraudNodes()
	return fg
}

// hashRecipient creates an anonymized node ID from a recipient identifier.
// The digest is truncated to 16 bytes (128-bit): at the 10k-node graph cap the
// birthday collision probability is negligible, versus ~1.5% at 8 bytes.
func hashRecipient(recipient string) string {
	r := strings.ToLower(strings.TrimSpace(recipient))
	h := sha256.Sum256([]byte(r))
	return hex.EncodeToString(h[:16])
}

// AddTransaction records a payment edge in the graph.
func (fg *FraudGraph) AddTransaction(from, to string, amountPaise int64) {
	fromID := hashRecipient(from)
	toID := hashRecipient(to)

	fg.mu.Lock()
	defer fg.mu.Unlock()

	if len(fg.nodes) >= maxGraphSize {
		fg.pruneOldNodes()
	}

	fromNode := fg.getOrCreateNode(fromID)
	fg.getOrCreateNode(toID)

	if fromNode.Edges == nil {
		fromNode.Edges = make(map[string]*GraphEdge)
	}

	now := time.Now().UTC()
	edge, exists := fromNode.Edges[toID]
	if !exists {
		edge = &GraphEdge{To: toID, FirstTxAt: now}
		fromNode.Edges[toID] = edge
		// Mirror into the reverse index (same pointer) so senders into toID are
		// discoverable in O(in-degree).
		if fg.inEdges[toID] == nil {
			fg.inEdges[toID] = make(map[string]*GraphEdge)
		}
		fg.inEdges[toID][fromID] = edge
	}
	edge.TxCount++
	edge.TotalAmt += amountPaise
	edge.LastTxAt = now
	// L-8 fix: weight now reflects cumulative amount and velocity, not just count,
	// so low-value structuring (many small transfers) can no longer stay below the
	// threshold.
	edge.Weight = structuringWeight(edge)
	fg.gen++
}

// MarkAsFraud marks a node as a confirmed fraud node.
func (fg *FraudGraph) MarkAsFraud(recipient string) {
	nodeID := hashRecipient(recipient)
	fg.mu.Lock()
	defer fg.mu.Unlock()

	node := fg.getOrCreateNode(nodeID)
	node.IsFraud = true
	node.RiskScore = fraudNodeRisk
	fg.gen++
}

// MarkAsMule marks a node as a suspected mule account.
func (fg *FraudGraph) MarkAsMule(recipient string) {
	nodeID := hashRecipient(recipient)
	fg.mu.Lock()
	defer fg.mu.Unlock()

	node := fg.getOrCreateNode(nodeID)
	node.IsMule = true
	node.RiskScore = math.Max(node.RiskScore, 0.8)
	fg.gen++
}

// GetRiskScore returns the GNN-style risk score for a recipient.
// This implements N-hop risk propagation from known fraud nodes.
//
// For an UNKNOWN recipient (not yet in the graph) it returns a weak, capped
// pattern-based prior. For a KNOWN node it returns the graph-derived risk only —
// the pattern score never overrides real graph structure. The top-level result
// is memoised per node and invalidated by the generation counter on any mutation.
func (fg *FraudGraph) GetRiskScore(recipient string) float64 {
	nodeID := hashRecipient(recipient)

	// Fast path: unknown node and cache lookup under the read lock.
	fg.mu.RLock()
	if _, exists := fg.nodes[nodeID]; !exists {
		fg.mu.RUnlock()
		return patternBasedScore(recipient)
	}
	if c, ok := fg.cache[nodeID]; ok && c.gen == fg.gen {
		fg.mu.RUnlock()
		return c.risk
	}
	fg.mu.RUnlock()

	// Slow path: compute and cache under the write lock (cache is shared state).
	fg.mu.Lock()
	defer fg.mu.Unlock()

	targetNode, exists := fg.nodes[nodeID]
	if !exists {
		// Node was pruned between the two critical sections.
		return patternBasedScore(recipient)
	}
	// Re-check the cache: another goroutine may have filled it while we waited.
	if c, ok := fg.cache[nodeID]; ok && c.gen == fg.gen {
		return c.risk
	}

	var risk float64
	switch {
	case targetNode.IsFraud:
		risk = fraudNodeRisk
	case targetNode.IsMule:
		risk = 0.8
	default:
		risk = fg.computeNHopRisk(nodeID, maxHops, make(map[string]bool))
	}

	fg.cache[nodeID] = cachedRisk{risk: risk, gen: fg.gen}
	return risk
}

// computeNHopRisk computes risk by propagating from fraud nodes through the graph.
// It must be called with the lock held. The visited set breaks cycles, which makes
// the result path-dependent — hence it is NOT memoised internally (only the
// top-level GetRiskScore result is cached).
func (fg *FraudGraph) computeNHopRisk(nodeID string, hopsRemaining int, visited map[string]bool) float64 {
	if hopsRemaining <= 0 || visited[nodeID] {
		return 0
	}

	node, exists := fg.nodes[nodeID]
	if !exists {
		return 0
	}

	visited[nodeID] = true
	defer delete(visited, nodeID)

	maxRisk := 0.0

	// Check direct (outbound) neighbors.
	for neighborID, edge := range node.Edges {
		neighbor, exists := fg.nodes[neighborID]
		if !exists {
			continue
		}

		var neighborRisk float64
		if neighbor.IsFraud {
			neighborRisk = fraudNodeRisk
		} else if neighbor.IsMule {
			neighborRisk = 0.8
		} else {
			// Recursively check further hops
			neighborRisk = fg.computeNHopRisk(neighborID, hopsRemaining-1, visited)
		}

		if neighborRisk > 0 {
			// Risk decays with hop distance and edge weight
			propagatedRisk := neighborRisk * riskDecayFactor * edge.Weight
			maxRisk = math.Max(maxRisk, propagatedRisk)
		}
	}

	// Check reverse edges (fraud/mule nodes sending TO this node) via the reverse
	// index — O(in-degree), not a full-graph scan.
	for senderID, edge := range fg.inEdges[nodeID] {
		sender, exists := fg.nodes[senderID]
		if !exists {
			continue
		}
		if !sender.IsFraud && !sender.IsMule {
			continue
		}
		baseRisk := 0.8
		if sender.IsFraud {
			baseRisk = fraudNodeRisk
		}
		propagatedRisk := baseRisk * riskDecayFactor * edge.Weight
		maxRisk = math.Max(maxRisk, propagatedRisk)
	}

	return maxRisk
}

// patternBasedScore provides a weak heuristic prior for recipients not yet in the
// graph. It matches on whole TOKENS (not substrings) so legitimate names that
// merely contain a keyword as a substring — "investment", "renew", "offerings" —
// do not trigger a false positive the way strings.Contains did. It is only ever
// used for unknown recipients and never overrides graph-derived risk.
func patternBasedScore(recipient string) float64 {
	r := strings.ToLower(strings.TrimSpace(recipient))
	if r == "" {
		return 0.5
	}

	tokens := tokenize(r)

	score := 0.0
	for _, tok := range tokens {
		if w, ok := riskKeywords[tok]; ok {
			score = math.Max(score, w)
		}
	}

	// Numeric-only or very short recipients are mildly suspicious.
	if len(r) <= 3 {
		score = math.Max(score, 0.4)
	}

	// Default low prior for normal-looking recipients.
	if score == 0 {
		score = 0.1
	}

	return score
}

// tokenize splits a recipient string into lowercase alphanumeric tokens.
func tokenize(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool {
		isLetter := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		return !isLetter && !isDigit
	})
}

// riskKeywords maps a scam/abuse token to a weight. High-risk scam terms score
// 0.9; softer solicitation terms score 0.5. Matched by exact token, not substring.
var riskKeywords = map[string]float64{
	// High-risk scam vocabulary.
	"unknown": 0.9, "urgent": 0.9, "lottery": 0.9, "prize": 0.9, "winner": 0.9,
	"free": 0.9, "gift": 0.9, "claim": 0.9, "verify": 0.9, "otp": 0.9,
	"hack": 0.9, "suspended": 0.9, "blocked": 0.9, "limited": 0.9,
	// Medium-risk solicitation vocabulary.
	"temp": 0.5, "test": 0.5, "demo": 0.5, "fake": 0.5,
	"invest": 0.5, "scheme": 0.5, "offer": 0.5, "deal": 0.5,
}

// getOrCreateNode gets or creates a graph node (must be called with write lock).
func (fg *FraudGraph) getOrCreateNode(nodeID string) *GraphNode {
	node, exists := fg.nodes[nodeID]
	if !exists {
		node = &GraphNode{
			ID:         nodeID,
			Edges:      make(map[string]*GraphEdge),
			LastUpdate: time.Now().UTC(),
		}
		fg.nodes[nodeID] = node
	}
	return node
}

// seedKnownFraudNodes pre-populates the graph with known fraud patterns.
func (fg *FraudGraph) seedKnownFraudNodes() {
	knownFraud := []string{
		"unknown-urgent", "lottery-winner", "free-prize",
		"fake-investment", "otp-scammer", "phishing-agent",
	}
	for _, fraud := range knownFraud {
		nodeID := hashRecipient(fraud)
		node := &GraphNode{
			ID:         nodeID,
			IsFraud:    true,
			RiskScore:  fraudNodeRisk,
			Edges:      make(map[string]*GraphEdge),
			LastUpdate: time.Now().UTC(),
		}
		fg.nodes[nodeID] = node
	}

	knownMules := []string{
		"mule-account-1", "mule-account-2", "mule-account-3",
		"rapid-withdrawal-node", "suspicious-beneficiary",
	}
	for _, mule := range knownMules {
		nodeID := hashRecipient(mule)
		node := &GraphNode{
			ID:         nodeID,
			IsMule:     true,
			RiskScore:  0.8,
			Edges:      make(map[string]*GraphEdge),
			LastUpdate: time.Now().UTC(),
		}
		fg.nodes[nodeID] = node
	}
}

// pruneOldNodes removes nodes that haven't been updated in 30 days. It must be
// called with the write lock held. Reverse-index and cache entries for pruned
// nodes are cleaned up so no stale pointers or stale scores survive.
func (fg *FraudGraph) pruneOldNodes() {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	pruned := false
	for id, node := range fg.nodes {
		if node.IsFraud || node.IsMule || !node.LastUpdate.Before(cutoff) {
			continue
		}
		// Remove this node's outbound edges from every target's reverse index.
		for toID := range node.Edges {
			if senders, ok := fg.inEdges[toID]; ok {
				delete(senders, id)
				if len(senders) == 0 {
					delete(fg.inEdges, toID)
				}
			}
		}
		// Remove this node's own reverse index and any forward edges pointing at it.
		for senderID := range fg.inEdges[id] {
			if sender, ok := fg.nodes[senderID]; ok {
				delete(sender.Edges, id)
			}
		}
		delete(fg.inEdges, id)
		delete(fg.cache, id)
		delete(fg.nodes, id)
		pruned = true
	}
	if pruned {
		fg.gen++
	}
}

// Softmax converts a slice of raw logits into a probability distribution that
// sums to 1. It is numerically stabilised by subtracting the max logit before
// exponentiating (prevents overflow on large logits).
//
// The mule-detection ONNX model (MuleAccountDetection.onnx) emits a raw
// per-node logits tensor of shape [num_nodes, 2] — NOT probabilities. The
// consumer must apply Softmax per row and take P(mule) = row[1] to obtain the
// RecipientGNNScore in [0,1] that feeds the transaction-risk model (see
// MODEL_IO_CONTRACT.md, two-stage pipeline). This helper lives here, alongside
// the hand-written GNN it mirrors, so both graph-risk paths share one softmax.
func Softmax(logits []float32) []float32 {
	if len(logits) == 0 {
		return nil
	}
	maxLogit := logits[0]
	for _, v := range logits[1:] {
		if v > maxLogit {
			maxLogit = v
		}
	}
	out := make([]float32, len(logits))
	var sum float64
	for i, v := range logits {
		e := math.Exp(float64(v - maxLogit))
		out[i] = float32(e)
		sum += e
	}
	if sum == 0 {
		return out
	}
	for i := range out {
		out[i] = float32(float64(out[i]) / sum)
	}
	return out
}

// GetStats returns statistics about the fraud graph.
func (fg *FraudGraph) GetStats() map[string]any {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	totalEdges := 0
	fraudNodes := 0
	muleNodes := 0
	for _, node := range fg.nodes {
		totalEdges += len(node.Edges)
		if node.IsFraud {
			fraudNodes++
		}
		if node.IsMule {
			muleNodes++
		}
	}

	return map[string]any{
		"total_nodes":  len(fg.nodes),
		"total_edges":  totalEdges,
		"fraud_nodes":  fraudNodes,
		"mule_nodes":   muleNodes,
		"max_hops":     maxHops,
		"decay_factor": riskDecayFactor,
	}
}
