#!/bin/bash
# Download the MiniLM ONNX model for embedding inference
# This script downloads the all-MiniLM-L6-v2 model from Hugging Face

set -e

MODEL_NAME="all-MiniLM-L6-v2"
MODEL_DIR="${HOME}/.switchailocal/models/${MODEL_NAME}"
MODEL_URL="https://huggingface.co/sentence-transformers/${MODEL_NAME}/resolve/main/onnx/model.onnx"
VOCAB_URL="https://huggingface.co/sentence-transformers/${MODEL_NAME}/resolve/main/vocab.txt"

echo "Downloading ${MODEL_NAME} ONNX model..."

# Create model directory
mkdir -p "${MODEL_DIR}"

# Download model file
if [ ! -f "${MODEL_DIR}/model.onnx" ]; then
    echo "Downloading model.onnx..."
    curl -L -o "${MODEL_DIR}/model.onnx" "${MODEL_URL}"
    echo "Model downloaded to ${MODEL_DIR}/model.onnx"
else
    echo "Model already exists at ${MODEL_DIR}/model.onnx"
fi

# Download vocabulary file
if [ ! -f "${MODEL_DIR}/vocab.txt" ]; then
    echo "Downloading vocab.txt..."
    curl -L -o "${MODEL_DIR}/vocab.txt" "${VOCAB_URL}"
    echo "Vocabulary downloaded to ${MODEL_DIR}/vocab.txt"
else
    echo "Vocabulary already exists at ${MODEL_DIR}/vocab.txt"
fi

echo ""
echo "Model files are ready at: ${MODEL_DIR}"
echo ""
echo "To use the embedding engine, add the following to your config.yaml:"
echo ""
echo "intelligence:"
echo "  enabled: true"
echo "  embedding:"
echo "    enabled: true"
echo "    model: ${MODEL_NAME}"
