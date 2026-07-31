//go:build !onnx

package fraud

import "fmt"

// MuleONNXPredictor stub for the default build (no `onnx` tag). It reports
// unavailable so ScoreMuleAccounts fails safe to the hand-written FraudGraph
// and `make test` needs no native ONNX Runtime. The real implementation is in
// mule_onnx.go (//go:build onnx).
type MuleONNXPredictor struct{}

func NewMuleONNXPredictor(modelPath string) *MuleONNXPredictor { return &MuleONNXPredictor{} }

func (m *MuleONNXPredictor) Predict(nodeFeatures []float32, numNodes int, edgeIndex []int64, numEdges int) ([]float32, error) {
	return nil, fmt.Errorf("mule onnx not available in this build")
}

func (m *MuleONNXPredictor) IsAvailable() bool { return false }
