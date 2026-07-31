Write-Host "Installing Python dependencies..."
pip install -r requirements.txt

# Only two models are served by the backend: the transaction risk score and the
# mule-account detector (see backend/models/model_registry.json).

Write-Host "Training transaction risk score model..."
python train_fraud_model.py
# -> ../models/security/transaction_risk_score/v1/{model.onnx, preprocess.joblib}

# NOTE: the mule-account detector's active path is the in-process structural GNN
# (internal/domain/fraud). The registry slot security/mule_account is reserved for
# the Stage-2 GCN, which needs a real labelled mule dataset + a train_mule_model.py
# before it can be exported here.

Write-Host "Models exported to backend/models/"
