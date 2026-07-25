#!/usr/bin/env python3
"""
FINIX ONNX Model Stub Generator
Generates minimal valid ONNX models for all 12 model registry entries.
These stubs produce deterministic outputs suitable for testing and development.

Usage:
    python scripts/generate_onnx_stubs.py
    python scripts/generate_onnx_stubs.py --output-dir ./models
"""

import argparse
import json
import os
import sys
from pathlib import Path

try:
    import onnx
    from onnx import helper, TensorProto
except ImportError:
    print("Installing onnx...")
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "onnx"])
    import onnx
    from onnx import helper, TensorProto


MODEL_SPECS = {
    "security/fraud_label/v1": {
        "input_features": 9,
        "output_classes": 3,
        "description": "Fraud label classifier (low/medium/high risk)"
    },
    "security/account_takeover_risk/v1": {
        "input_features": 7,
        "output_classes": 2,
        "description": "Account takeover risk binary classifier"
    },
    "security/beneficiary_abuse_risk/v1": {
        "input_features": 5,
        "output_classes": 2,
        "description": "Beneficiary abuse risk detector"
    },
    "security/action_class/v1": {
        "input_features": 8,
        "output_classes": 4,
        "description": "Action classification (allow/step_up/block/escalate)"
    },
    "behaviour/user_segment/v1": {
        "input_features": 6,
        "output_classes": 4,
        "description": "User segment classifier"
    },
    "behaviour/behaviour_stability_band/v1": {
        "input_features": 5,
        "output_classes": 1,
        "description": "Behaviour stability score regressor"
    },
    "behaviour/spending_discipline_band/v1": {
        "input_features": 5,
        "output_classes": 1,
        "description": "Spending discipline score regressor"
    },
    "behaviour/goal_completion_band/v1": {
        "input_features": 6,
        "output_classes": 1,
        "description": "Goal completion probability regressor"
    },
    "investment/risk_profile/v1": {
        "input_features": 7,
        "output_classes": 3,
        "description": "Investment risk profile classifier"
    },
    "investment/sip_action/v1": {
        "input_features": 5,
        "output_classes": 3,
        "description": "SIP action recommendation"
    },
    "investment/goal_priority/v1": {
        "input_features": 4,
        "output_classes": 3,
        "description": "Goal priority classifier"
    },
    "investment/asset_mix_template/v1": {
        "input_features": 6,
        "output_classes": 4,
        "description": "Asset mix template selector"
    },
}


def make_onnx_model(input_features: int, output_classes: int, model_name: str):
    """Create a minimal ONNX model with a single linear layer (identity-ish)."""
    
    input_shape = [1, input_features]
    
    # Input
    X = helper.make_tensor_value_info("float_input", TensorProto.FLOAT, input_shape)
    
    # Weight matrix: identity-like (zeros with 1s on first diagonal overlap)
    import numpy as np
    W_data = np.eye(input_features, output_classes, dtype=np.float32) * 0.1
    B_data = np.zeros(output_classes, dtype=np.float32)
    
    W = helper.make_tensor(
        name="linear.weight",
        data_type=TensorProto.FLOAT,
        dims=[output_classes, input_features],
        vals=W_data.flatten().tolist(),
    )
    B = helper.make_tensor(
        name="linear.bias",
        data_type=TensorProto.FLOAT,
        dims=[output_classes],
        vals=B_data.tolist(),
    )
    
    # Softmax output for classification, identity for regression
    if output_classes == 1:
        Y = helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 1])
        matmul = helper.make_node("Gemm", ["float_input", "linear.weight", "linear.bias"], ["gemm_output"], name="gemm")
        graph = helper.make_graph(
            nodes=[matmul],
            name=model_name,
            inputs=[X],
            outputs=[Y],
            initializer=[W, B],
        )
    else:
        Y = helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, output_classes])
        matmul = helper.make_node("Gemm", ["float_input", "linear.weight", "linear.bias"], ["gemm_output"], name="gemm")
        softmax = helper.make_node("Softmax", ["gemm_output"], ["output"], name="softmax")
        graph = helper.make_graph(
            nodes=[matmul, softmax],
            name=model_name,
            inputs=[X],
            outputs=[Y],
            initializer=[W, B],
        )
    
    model = helper.make_model(graph, producer_name="FINIX-stub-generator", opset_imports=[helper.make_opsetid("", 13)])
    return model


def main():
    parser = argparse.ArgumentParser(description="Generate ONNX stub models for FINIX")
    parser.add_argument("--output-dir", default="./models", help="Output directory (default: ./models)")
    args = parser.parse_args()
    
    base_dir = Path(args.output_dir)
    
    for model_path, spec in MODEL_SPECS.items():
        full_path = base_dir / model_path
        full_path.mkdir(parents=True, exist_ok=True)
        
        model_file = full_path / "model.onnx"
        
        if model_file.exists():
            print(f"SKIP (exists): {model_file}")
            continue
        
        model = make_onnx_model(spec["input_features"], spec["output_classes"], model_path.replace("/", "_"))
        onnx.save_model(model, str(model_file))
        
        # Write metadata
        meta = {
            "model": model_path,
            "input_features": spec["input_features"],
            "output_classes": spec["output_classes"],
            "description": spec["description"],
            "version": "v1",
            "generated_by": "stub_generator",
        }
        with open(full_path / "metadata.json", "w") as f:
            json.dump(meta, f, indent=2)
        
        print(f"CREATED: {model_file} ({spec['input_features']}→{spec['output_classes']})")
    
    print(f"\nDone. Generated {len(MODEL_SPECS)} ONNX models in {base_dir.absolute()}")


if __name__ == "__main__":
    main()
