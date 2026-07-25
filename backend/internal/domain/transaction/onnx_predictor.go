//go:build onnx

package transaction

import (
	"fmt"
	"log/slog"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

type ONNXPredictor struct {
	mu        sync.RWMutex
	session   *ort.OnnxSession
	available bool
	inputName string
}

func NewONNXPredictor(modelPath string) *ONNXPredictor {
	if modelPath == "" {
		slog.Warn("onnx: no model path provided, disabling ONNX predictor")
		return &ONNXPredictor{available: false}
	}

	err := ort.InitializeEnvironment()
	if err != nil {
		slog.Warn("onnx: failed to initialize runtime", "error", err)
		return &ONNXPredictor{available: false}
	}

	session, err := ort.NewSession(modelPath)
	if err != nil {
		slog.Warn("onnx: failed to load model", "path", modelPath, "error", err)
		return &ONNXPredictor{available: false}
	}

	inputNames := session.GetInputNames()
	if len(inputNames) == 0 {
		slog.Warn("onnx: model has no inputs", "path", modelPath)
		session.Destroy()
		return &ONNXPredictor{available: false}
	}

	slog.Info("onnx: model loaded", "path", modelPath, "input", inputNames[0])
	return &ONNXPredictor{
		session:   session,
		available: true,
		inputName: inputNames[0],
	}
}

func (o *ONNXPredictor) Predict(features []float32) ([]float32, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if !o.available || o.session == nil {
		return nil, fmt.Errorf("onnx predictor not available")
	}

	input, err := ort.NewTensorFromFloat32Bytes(features, []int64{1, int64(len(features))})
	if err != nil {
		return nil, fmt.Errorf("onnx: failed to create input tensor: %w", err)
	}
	defer input.Destroy()

	outputs, err := o.session.Run(nil, []ort.Value{input})
	if err != nil {
		return nil, fmt.Errorf("onnx: inference failed: %w", err)
	}

	if len(outputs) == 0 {
		return nil, fmt.Errorf("onnx: model returned no outputs")
	}

	outputTensor, ok := outputs[0].(*ort.TensorFloat32)
	if !ok {
		return nil, fmt.Errorf("onnx: unexpected output type")
	}

	outputData := outputTensor.GetData()
	if len(outputData) < 3 {
		return nil, fmt.Errorf("onnx: expected 3 output classes, got %d", len(outputData))
	}

	result := make([]float32, 3)
	copy(result, outputData[:3])
	return result, nil
}

func (o *ONNXPredictor) IsAvailable() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.available
}

func (o *ONNXPredictor) Destroy() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.session != nil {
		o.session.Destroy()
		o.session = nil
	}
	o.available = false
}
