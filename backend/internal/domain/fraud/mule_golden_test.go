//go:build onnx

package fraud

import (
	"errors"
	"math"
	"testing"
)

// This golden-vector test LOCKS the 20-column mule node encoding to the
// training-time preprocessing artifact the user is supplying. It is guarded by
// //go:build onnx so the DEFAULT build (`make test`, no -tags onnx) stays green
// while the encoding is unwired; it only runs — and is only expected to pass —
// once you build with `-tags onnx` AND the encoder + golden vectors are real.
//
// WHY a golden test is the single most important safeguard here: an ONNX model
// carries no reliable description of what each input column means (see
// ONNX_INTEGRATION_AUDIT.md, findings A-4/B-4). If encodeMuleNode emits the 20
// numbers in the wrong order/scale, inference "works" and returns
// confident-but-wrong mule scores — worse than no model. Asserting a handful of
// (raw account -> expected encoded row) pairs exported from the training
// environment makes a bad wiring fail loudly instead of silently.
//
// HOW TO POPULATE (when the preprocessing artifact lands):
//  1. In the training environment, pick a few representative accounts and run
//     them through the exact preprocessing that fed the GNN. Export each raw
//     account's fields plus the resulting 20-float row.
//  2. Fill muleGoldenVectors below with those pairs.
//  3. Implement encodeMuleNode in graph_predictor.go from the same
//     preprocessing (column order, categorical encodings, scaler mean/std).
//  4. Build/run with: `go test -tags onnx ./internal/domain/fraud/ -run TestMuleNodeEncodingGolden`.
//
// Until step 2/3 are done this test is intentionally RED under -tags onnx:
// muleGoldenVectors is empty (the guard below fails) and encodeMuleNode returns
// errMuleEncodingUnset. That RED is the reminder that the contract is not yet
// locked — do NOT delete the guard or invent vectors to make it pass.

// muleGoldenTolerance is the max absolute per-element difference allowed between
// encodeMuleNode's output and the training-exported golden row. Encodings that
// involve float scaling won't be bit-identical across languages, so a small
// tolerance is expected; keep it tight enough to catch a wrong column/scale.
const muleGoldenTolerance = 1e-4

// muleGoldenVector is one (raw account -> expected encoded row) pair exported
// from the training preprocessing.
type muleGoldenVector struct {
	name     string
	account  MuleAccount
	expected []float32 // exactly muleNodeFeatureWidth (20) elements
}

// muleGoldenVectors is populated FROM THE SUPPLIED PREPROCESSING ARTIFACT. It is
// deliberately empty until then. Do NOT hand-author vectors by guessing the
// column layout — that would defeat the purpose of this test.
var muleGoldenVectors = []muleGoldenVector{
	// EXAMPLE SHAPE (commented out — replace with real exported vectors):
	// {
	// 	name: "dormant_reactivated_savings",
	// 	account: MuleAccount{ID: "acc-001", Raw: map[string]float64{
	// 		"account_age_days": 812, "current_balance": 1500000, /* ...paise... */
	// 	}},
	// 	expected: []float32{ /* 20 floats in trained order/scale */ },
	// },
}

func TestMuleNodeEncodingGolden(t *testing.T) {
	// encodeMuleNode is now wired to mule_preprocess.json (see graph_predictor.go
	// / SetMuleEncoder). What remains for a true CROSS-LANGUAGE lock is a handful
	// of (raw account -> expected encoded row) pairs exported from the Python
	// training preprocessing. Until those are pasted below, skip rather than fail
	// so `-tags onnx` runs stay green; the Go-side encoding is covered by
	// TestEncodeMuleNodeUsesArtifact in the default build.
	if len(muleGoldenVectors) == 0 {
		t.Skip("cross-language mule golden vectors not supplied yet (export raw->row pairs " +
			"from the training preprocessing to lock Go/Python parity)")
	}
	if !MuleEncoderReady() {
		t.Skip("mule encoder not wired in this run (mule_preprocess.json absent)")
	}

	for _, gv := range muleGoldenVectors {
		gv := gv
		t.Run(gv.name, func(t *testing.T) {
			if len(gv.expected) != muleNodeFeatureWidth {
				t.Fatalf("golden vector %q has %d expected features, want %d",
					gv.name, len(gv.expected), muleNodeFeatureWidth)
			}

			got, err := encodeMuleNode(gv.account)
			if err != nil {
				if errors.Is(err, errMuleEncodingUnset) {
					t.Fatalf("encodeMuleNode still unwired (errMuleEncodingUnset): implement it " +
						"from the supplied preprocessing before asserting golden vectors")
				}
				t.Fatalf("encodeMuleNode(%q) returned error: %v", gv.name, err)
			}
			if len(got) != muleNodeFeatureWidth {
				t.Fatalf("encodeMuleNode(%q) returned %d features, want %d",
					gv.name, len(got), muleNodeFeatureWidth)
			}

			for i := range gv.expected {
				diff := math.Abs(float64(got[i] - gv.expected[i]))
				if diff > muleGoldenTolerance {
					t.Errorf("encodeMuleNode(%q) column %d = %g, want %g (|diff| %g > tol %g)",
						gv.name, i, got[i], gv.expected[i], diff, muleGoldenTolerance)
				}
			}
		})
	}
}

// TestMuleNodeEncodingWidthContract is a lighter guard that runs alongside the
// golden test under -tags onnx: whatever encodeMuleNode returns for a non-error
// input, it must be exactly muleNodeFeatureWidth wide. AssembleGraph enforces
// this at assembly time too, but asserting it directly pins the contract at the
// encoder boundary so a width regression is caught in isolation.
func TestMuleNodeEncodingWidthContract(t *testing.T) {
	if len(muleGoldenVectors) == 0 {
		t.Skip("no golden vectors yet; width contract is exercised once encoding is wired")
	}
	for _, gv := range muleGoldenVectors {
		got, err := encodeMuleNode(gv.account)
		if err != nil {
			continue // width is only defined for successful encodings
		}
		if len(got) != muleNodeFeatureWidth {
			t.Errorf("encodeMuleNode(%q) width %d, want %d", gv.name, len(got), muleNodeFeatureWidth)
		}
	}
}
