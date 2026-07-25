Write-Host "Installing Python dependencies..."
pip install -r requirements.txt

Write-Host "Training fraud detection model..."
python train_fraud_model.py

Write-Host "Training behaviour segmentation model..."
python train_behaviour_model.py

Write-Host "Models exported to backend/models/"
