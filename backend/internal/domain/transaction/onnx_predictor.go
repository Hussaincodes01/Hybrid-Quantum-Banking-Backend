//go:build onnx

package transaction

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// probabilitiesOutput is the tensor name the transaction_risk_model.onnx graph
// exposes for the class-probability vector. The graph also exposes an int64
// "label" output at index 0; reading that as float32 is the A-3 bug this file
// previously had. We select the float probabilities output by name instead.
const probabilitiesOutput = "probabilities"

type ONNXPredictor struct {
	mu          sync.RWMutex
	session     *ort.OnnxSession
	available   bool
	inputName   string
	outputNames []string
	// probIdx is the position of the "probabilities" output in outputNames,
	// or -1 if the model does not expose a named probabilities output.
	probIdx int
}

func NewONNXPredictor(modelPath string) *ONNXPredictor {
	if modelPath == "" {
		slog.Warn("onnx: no model path provided, disabling ONNX predictor")
		return &ONNXPredictor{available: false}
	}

	// yalue/onnxruntime_go is a cgo wrapper over the native ONNX Runtime
	// shared library; it must be locatable before InitializeEnvironment.
	// FINIX_ONNXRUNTIME_LIB lets ops point at the vendored .dll/.so per OS
	// (finding C-1 in the integration audit). If unset we let the library
	// attempt its own default discovery.
	if libPath := strings.TrimSpace(os.Getenv("FINIX_ONNXRUNTIME_LIB")); libPath != "" {
		ort.SetSharedLibraryPath(libPath)
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

	outputNames := session.GetOutputNames()
	probIdx := -1
	for i, name := range outputNames {
		if name == probabilitiesOutput {
			probIdx = i
			break
		}
	}
	if probIdx < 0 {
		slog.Warn("onnx: model has no 'probabilities' output; disabling",
			"path", modelPath, "outputs", outputNames)
		session.Destroy()
		return &ONNXPredictor{available: false}
	}

	slog.Info("onnx: model loaded", "path", modelPath,
		"input", inputNames[0], "prob_output", probabilitiesOutput, "prob_index", probIdx)
	return &ONNXPredictor{
		session:     session,
		available:   true,
		inputName:   inputNames[0],
		outputNames: outputNames,
		probIdx:     probIdx,
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

	// Select the float "probabilities" output by index, NOT outputs[0]:
	// transaction_risk_model.onnx exposes an int64 "label" at index 0 and the
	// float32 "probabilities" at o.probIdx. Reading outputs[0] as float32 was
	// the A-3 bug (silent type mismatch / garbage scores).
	if o.probIdx >= len(outputs) {
		return nil, fmt.Errorf("onnx: probabilities output index %d out of range (%d outputs)", o.probIdx, len(outputs))
	}
	defer func() {
		for _, out := range outputs {
			if out != nil {
				out.Destroy()
			}
		}
	}()

	outputTensor, ok := outputs[o.probIdx].(*ort.TensorFloat32)
	if !ok {
		return nil, fmt.Errorf("onnx: 'probabilities' output is not float32")
	}

	// Binary classifier: probabilities is [N, 2] = [P(legit), P(fraud)].
	// For a single-row inference (N=1) that is exactly 2 values. Return the
	// two-class vector as-is; assessmentFromProbs interprets [pLegit, pFraud].
	outputData := outputTensor.GetData()
	if len(outputData) < 2 {
		return nil, fmt.Errorf("onnx: expected 2 output classes, got %d", len(outputData))
	}

	result := make([]float32, 2)
	copy(result, outputData[:2])
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
