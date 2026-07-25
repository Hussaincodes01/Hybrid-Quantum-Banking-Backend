//go:build !onnx

package transaction

import "fmt"

type ONNXPredictor struct{}

func NewONNXPredictor(modelPath string) *ONNXPredictor { return &ONNXPredictor{} }

func (o *ONNXPredictor) Predict(features []float32) ([]float32, error) {
	return nil, fmt.Errorf("onnx not available in this build")
}

func (o *ONNXPredictor) IsAvailable() bool { return false }
