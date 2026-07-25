package transaction

// ModelPredictor is the interface for ML model inference.
type ModelPredictor interface {
	Predict(features []float32) ([]float32, error)
	IsAvailable() bool
}
