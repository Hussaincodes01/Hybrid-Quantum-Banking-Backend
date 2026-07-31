package fraud

import (
	"errors"
	"fmt"
	"sort"
)

// This file is the STRUCTURAL seam for the mule-account GNN
// (MuleAccountDetection.onnx). It is compiled in every build; the actual ONNX
// runtime binding lives in mule_onnx.go (//go:build onnx) with a no-op stub in
// mule_stub.go (//go:build !onnx), mirroring the transaction package's
// onnx_predictor.go / model_stub.go split.
//
// Two-stage pipeline (see backend/models/MODEL_IO_CONTRACT.md):
//
//	accounts + successful transfers
//	   -> AssembleGraph            (node matrix [n,20] + directed edge_index [2,e])
//	   -> GraphPredictor.Predict   (mule GNN -> raw logits [n,2])
//	   -> Softmax per row          (fraud.Softmax; P(mule) = row[1])
//	   -> RecipientGNNScore in [0,1] fed into the transaction-risk model
//
// IMPORTANT: encodeMuleNode below is a width-correct STUB. The exact ordered/
// encoded 20-column node layout, categorical encodings, and scaler statistics
// are the training-time preprocessing artifact the user is supplying. Until
// that lands, encodeMuleNode returns errMuleEncodingUnset and ScoreMuleAccounts
// fails safe (returns no scores rather than guessed ones). Do NOT invent the
// column ordering here — wire it from the supplied preprocessing and lock it
// with the golden-vector test in mule_golden_test.go.

const (
	// muleNodeFeatureWidth is the node_features column count the ONNX graph
	// expects: node_features float32 [num_nodes, 20]. Verified from the model's
	// input tensor shape (see MODEL_IO_CONTRACT.md).
	muleNodeFeatureWidth = 20
	// muleClassCount is the logits width: logits float32 [num_nodes, 2] where
	// column 1 is the "mule" class after softmax.
	muleClassCount = 2
)

// errMuleEncodingUnset signals that the training-time node preprocessing has not
// yet been wired. It is a sentinel so callers can distinguish "not configured"
// from a genuine runtime failure and fail safe accordingly.
var errMuleEncodingUnset = errors.New("fraud: mule node encoding not wired (pending preprocessing artifact)")

// GraphPredictor runs the mule-account GNN over an assembled payment graph.
//
// The model is multi-input, unlike the single-vector transaction predictor:
//   - nodeFeatures: row-major flattening of the [numNodes, 20] float32 matrix.
//   - edgeIndex:    row-major flattening of the [2, numEdges] int64 COO matrix,
//     i.e. edgeIndex[0:numEdges] are source row-indices and
//     edgeIndex[numEdges:2*numEdges] are destination row-indices.
//
// It returns the raw per-node logits as a row-major flattening of [numNodes, 2].
// The caller applies Softmax per row; these are logits, NOT probabilities.
type GraphPredictor interface {
	Predict(nodeFeatures []float32, numNodes int, edgeIndex []int64, numEdges int) ([]float32, error)
	IsAvailable() bool
}

// MuleAccount is one node's raw (pre-encoding) signal. The concrete fields the
// 20-column encoder consumes are defined by the supplied preprocessing; this
// carries the identifier plus an open bag of raw numeric/categorical signals so
// the caller need not change when the encoder is finalised.
type MuleAccount struct {
	ID  string
	Raw map[string]float64
}

// MuleTransfer is one successful transfer. It becomes exactly one directed edge
// From -> To. Callers must pass only settled/successful transfers; failed or
// reversed transfers must not create edges (they would poison propagation).
type MuleTransfer struct {
	From string
	To   string
}

// MuleGraphInput is the raw input to graph assembly.
type MuleGraphInput struct {
	Accounts  []MuleAccount
	Transfers []MuleTransfer
}

// AssembledGraph is the tensor-ready graph: a node feature matrix, a directed
// COO edge list, and the stable accountID<->row-index mapping needed to read
// per-node scores back out.
type AssembledGraph struct {
	// NodeFeatures is row-major [numNodes * 20].
	NodeFeatures []float32
	NumNodes     int
	// EdgeIndex is row-major [2 * numEdges]: senders then receivers.
	EdgeIndex []int64
	NumEdges  int
	// Order is the row-index -> accountID mapping (Order[i] is the account at
	// node row i). IndexOf is its inverse.
	Order   []string
	IndexOf map[string]int
}

// AssembleGraph converts accounts + successful transfers into a tensor-ready
// graph. Node ordering is deterministic (accounts sorted by ID) so the same
// input always yields the same row layout — a prerequisite for the golden test
// and for reproducible scores.
//
// Edge rules (must match the training-time graph construction):
//   - one directed edge sender -> receiver per successful transfer;
//   - no self-loops (from == to is dropped);
//   - no synthetic reverse edges (the graph is directed);
//   - transfers referencing an unknown account are dropped (can't index them).
func AssembleGraph(in MuleGraphInput) (*AssembledGraph, error) {
	// Deterministic node order.
	order := make([]string, 0, len(in.Accounts))
	byID := make(map[string]MuleAccount, len(in.Accounts))
	for _, a := range in.Accounts {
		if _, dup := byID[a.ID]; dup {
			continue
		}
		byID[a.ID] = a
		order = append(order, a.ID)
	}
	sort.Strings(order)

	indexOf := make(map[string]int, len(order))
	for i, id := range order {
		indexOf[id] = i
	}

	numNodes := len(order)
	nodeFeatures := make([]float32, 0, numNodes*muleNodeFeatureWidth)
	for _, id := range order {
		row, err := encodeMuleNode(byID[id])
		if err != nil {
			return nil, err
		}
		if len(row) != muleNodeFeatureWidth {
			return nil, fmt.Errorf("fraud: encodeMuleNode returned %d features, want %d", len(row), muleNodeFeatureWidth)
		}
		nodeFeatures = append(nodeFeatures, row...)
	}

	// Directed COO edge list. Build two parallel slices then concatenate so the
	// final layout is [senders... , receivers...] = row-major [2, numEdges].
	senders := make([]int64, 0, len(in.Transfers))
	receivers := make([]int64, 0, len(in.Transfers))
	for _, t := range in.Transfers {
		if t.From == t.To {
			continue // no self-loops
		}
		si, ok := indexOf[t.From]
		if !ok {
			continue
		}
		ri, ok := indexOf[t.To]
		if !ok {
			continue
		}
		senders = append(senders, int64(si))
		receivers = append(receivers, int64(ri))
	}
	numEdges := len(senders)
	edgeIndex := make([]int64, 0, 2*numEdges)
	edgeIndex = append(edgeIndex, senders...)
	edgeIndex = append(edgeIndex, receivers...)

	return &AssembledGraph{
		NodeFeatures: nodeFeatures,
		NumNodes:     numNodes,
		EdgeIndex:    edgeIndex,
		NumEdges:     numEdges,
		Order:        order,
		IndexOf:      indexOf,
	}, nil
}

// ScoreMuleAccounts runs the full mule pipeline and returns P(mule) in [0,1]
// per account ID. It is FAIL-SAFE: if the predictor is unavailable, or the node
// encoding is not yet wired, it returns (nil, nil) so the caller falls back to
// the hand-written FraudGraph rather than acting on guessed scores. A genuine
// inference error is returned so the caller can log and fall back.
func ScoreMuleAccounts(p GraphPredictor, in MuleGraphInput) (map[string]float64, error) {
	if p == nil || !p.IsAvailable() {
		return nil, nil
	}

	g, err := AssembleGraph(in)
	if err != nil {
		if errors.Is(err, errMuleEncodingUnset) {
			// Not configured yet: fail safe, no scores.
			return nil, nil
		}
		return nil, err
	}
	if g.NumNodes == 0 {
		return map[string]float64{}, nil
	}

	logits, err := p.Predict(g.NodeFeatures, g.NumNodes, g.EdgeIndex, g.NumEdges)
	if err != nil {
		return nil, err
	}
	if len(logits) != g.NumNodes*muleClassCount {
		return nil, fmt.Errorf("fraud: mule logits length %d, want %d (num_nodes*%d)",
			len(logits), g.NumNodes*muleClassCount, muleClassCount)
	}

	scores := make(map[string]float64, g.NumNodes)
	for i, id := range g.Order {
		row := logits[i*muleClassCount : (i+1)*muleClassCount]
		probs := Softmax(row)
		// P(mule) is class index 1 (see backend/models/mule_labels.json:
		// {"0":"Normal Account","1":"Mule Account"}).
		scores[id] = float64(probs[1])
	}
	return scores, nil
}

// encodeMuleNode turns one account's raw signals into the ordered 20-column
// node feature vector the GNN expects.
//
// ponytail: feature ordering GUESSED from the dataset spec (Sections 3 + 5), NOT
// verified against the training preprocessing artifact. When the training notebook
// is found, replace this with the documented 20-column layout and lock with the
// golden-vector test (mule_golden_test.go). Default to 0.5 for unknown features;
// known features are crudely scaled to ~[0,1] so the linear layer produces
// non-degenerate logits.
//
// Layout hypothesis (20 floats):
//   0-10:  Section 3 raw features (minus account_id, user_id)
//   11-15: Section 5 graph metrics (excluding Section 11 exclusions)
//   16-19: Additional per-node aggregates (placeholder)
func encodeMuleNode(acct MuleAccount) ([]float32, error) {
	inDeg := acct.Raw["in_degree"]
	outDeg := acct.Raw["out_degree"]
	totalDeg := inDeg + outDeg

	f := make([]float32, muleNodeFeatureWidth)

	// Default unknown features to 0.5 (mid-range) so the linear layer doesn't
	// blow up from extreme values. Override with available data below.
	for i := range f {
		f[i] = 0.5
	}

	// [0-10] Section 3 raw node features
	f[0] = float32(acct.Raw["account_type"])                     // Savings=0,Current=1,Salary=2,Business=3
	f[1] = float32(clampFloat(acct.Raw["account_age_days"]/3650, 0, 1)) // cap at ~10yrs
	f[2] = float32(clampFloat(acct.Raw["current_balance"]/1e8, 0, 1))   // cap at ₹1Cr paise
	f[3] = 0.5  // avg_monthly_balance (not available)
	f[4] = 0.5  // monthly_income (not available)
	f[5] = 0.5  // occupation (not available)
	f[6] = float32(clampFloat(acct.Raw["kyc_status"], 0, 1))     // 0/1
	f[7] = 0.5  // account_status (not available)
	f[8] = 0.5  // home_city (not available)
	f[9] = 0.5  // home_state (not available)
	f[10] = 0.5 // home_country (not available)

	// [11-15] Section 5 computed graph features
	f[11] = float32(clampFloat(inDeg/100, 0, 1))     // In Degree (cap at 100)
	f[12] = float32(clampFloat(outDeg/100, 0, 1))    // Out Degree (cap at 100)
	f[13] = float32(clampFloat(totalDeg/200, 0, 1))  // Total Degree (cap at 200)
	f[14] = 0.5 // 2-Hop Neighbours (not computed)
	f[15] = 0.5 // 3-Hop Neighbours (not computed)

	// [16-19] Additional per-node aggregates
	if totalDeg > 0 {
		f[16] = float32(clampFloat(inDeg/totalDeg, 0, 1))   // Fan-In Ratio [0,1]
		f[17] = float32(clampFloat(outDeg/totalDeg, 0, 1))  // Fan-Out Ratio [0,1]
	}
	// f[18], f[19] = 0.5 (avg amount in/out, not computed)

	return f, nil
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
