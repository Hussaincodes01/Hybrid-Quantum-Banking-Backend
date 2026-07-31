//go:build onnx

package fraud

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Mule GNN ONNX tensor contract (verified; see MODEL_IO_CONTRACT.md):
//
//	INPUTS
//	  node_features  float32 [num_nodes, 20]
//	  edge_index     int64   [2, num_edges]   (row 0 = senders, row 1 = receivers)
//	OUTPUT
//	  logits         float32 [num_nodes, 2]   (RAW logits; softmax downstream)
//
// Producer pytorch, opset 18. Names are matched by string, not position, so a
// re-export that reorders I/O does not silently break inference.
const (
	muleNodeInputName  = "node_features"
	muleEdgeInputName  = "edge_index"
	muleLogitsOutput   = "logits"
)

// MuleONNXPredictor is the //go:build onnx implementation of GraphPredictor.
// The parallel //go:build !onnx stub in mule_stub.go reports unavailable so the
// default build (and `make test`, which runs without -tags onnx) needs no
// native ONNX Runtime and falls back to the hand-written FraudGraph.
type MuleONNXPredictor struct {
	mu        sync.RWMutex
	session   *ort.DynamicAdvancedSession
	available bool
	nodeIn    string
	edgeIn    string
	logitsOut string
}

func NewMuleONNXPredictor(modelPath string) *MuleONNXPredictor {
	if modelPath == "" {
		slog.Warn("mule onnx: no model path provided, disabling predictor")
		return &MuleONNXPredictor{available: false}
	}

	if libPath := strings.TrimSpace(os.Getenv("FINIX_ONNXRUNTIME_LIB")); libPath != "" {
		ort.SetSharedLibraryPath(libPath)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		slog.Warn("mule onnx: failed to initialize runtime", "error", err)
		return &MuleONNXPredictor{available: false}
	}

	// Verify the model exposes the expected multi-input / single-output names
	// before committing to a session, so a contract mismatch disables the
	// predictor (safe heuristic fallback) instead of erroring at inference.
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		slog.Warn("mule onnx: failed to inspect model", "path", modelPath, "error", err)
		return &MuleONNXPredictor{available: false}
	}
	haveNode, haveEdge := false, false
	for _, in := range inputs {
		switch in.Name {
		case muleNodeInputName:
			haveNode = true
		case muleEdgeInputName:
			haveEdge = true
		}
	}
	haveLogits := false
	for _, out := range outputs {
		if out.Name == muleLogitsOutput {
			haveLogits = true
		}
	}
	if !haveNode || !haveEdge || !haveLogits {
		slog.Warn("mule onnx: model I/O names do not match contract; disabling",
			"path", modelPath, "want_inputs", []string{muleNodeInputName, muleEdgeInputName},
			"want_output", muleLogitsOutput)
		return &MuleONNXPredictor{available: false}
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{muleNodeInputName, muleEdgeInputName},
		[]string{muleLogitsOutput}, nil)
	if err != nil {
		slog.Warn("mule onnx: failed to load model", "path", modelPath, "error", err)
		return &MuleONNXPredictor{available: false}
	}

	slog.Info("mule onnx: model loaded", "path", modelPath)
	return &MuleONNXPredictor{
		session:   session,
		available: true,
		nodeIn:    muleNodeInputName,
		edgeIn:    muleEdgeInputName,
		logitsOut: muleLogitsOutput,
	}
}

func (m *MuleONNXPredictor) Predict(nodeFeatures []float32, numNodes int, edgeIndex []int64, numEdges int) ([]float32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.available || m.session == nil {
		return nil, fmt.Errorf("mule onnx predictor not available")
	}
	if numNodes <= 0 {
		return nil, fmt.Errorf("mule onnx: numNodes must be > 0, got %d", numNodes)
	}
	if len(nodeFeatures) != numNodes*muleNodeFeatureWidth {
		return nil, fmt.Errorf("mule onnx: node features length %d, want %d",
			len(nodeFeatures), numNodes*muleNodeFeatureWidth)
	}
	if len(edgeIndex) != 2*numEdges {
		return nil, fmt.Errorf("mule onnx: edge index length %d, want %d", len(edgeIndex), 2*numEdges)
	}

	nodeTensor, err := ort.NewTensorFromFloat32Bytes(nodeFeatures,
		[]int64{int64(numNodes), int64(muleNodeFeatureWidth)})
	if err != nil {
		return nil, fmt.Errorf("mule onnx: failed to create node tensor: %w", err)
	}
	defer nodeTensor.Destroy()

	// edge_index is [2, num_edges]; a graph with zero edges is still valid
	// (isolated nodes), and the tensor shape stays [2, 0].
	edgeTensor, err := ort.NewTensor([]int64{2, int64(numEdges)}, edgeIndex)
	if err != nil {
		return nil, fmt.Errorf("mule onnx: failed to create edge tensor: %w", err)
	}
	defer edgeTensor.Destroy()

	outputs := []ort.Value{nil}
	if err := m.session.Run([]ort.Value{nodeTensor, edgeTensor}, outputs); err != nil {
		return nil, fmt.Errorf("mule onnx: inference failed: %w", err)
	}
	defer func() {
		for _, out := range outputs {
			if out != nil {
				out.Destroy()
			}
		}
	}()

	logitsTensor, ok := outputs[0].(*ort.TensorFloat32)
	if !ok {
		return nil, fmt.Errorf("mule onnx: 'logits' output is not float32")
	}
	data := logitsTensor.GetData()
	if len(data) != numNodes*muleClassCount {
		return nil, fmt.Errorf("mule onnx: logits length %d, want %d", len(data), numNodes*muleClassCount)
	}
	result := make([]float32, len(data))
	copy(result, data)
	return result, nil
}

func (m *MuleONNXPredictor) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available
}

func (m *MuleONNXPredictor) Destroy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session != nil {
		m.session.Destroy()
		m.session = nil
	}
	m.available = false
}
