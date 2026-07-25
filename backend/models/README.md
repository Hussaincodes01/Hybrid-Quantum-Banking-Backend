# Model Artifact Layout

This directory stores trained model artifacts (weights) and preprocessing objects.

## Conventions
- One folder per task.
- Versioned subfolder for each released model.
- Keep model file and preprocessing file(s) together.
- Update model_registry.json when promoting a new version.

## Current serving baseline
The current backend AIML behavior is heuristic/rule-based in service code.
Use this folder now to stage trained artifacts for the next integration step.
