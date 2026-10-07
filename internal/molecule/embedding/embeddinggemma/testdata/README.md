# Model Test Data

Transformers.js gives reference vectors. The Go adapter does not make the reference vectors.

```text
Package: @huggingface/transformers 4.3.1
Backend: onnxruntime-node 1.30.0, CPU
Model: onnx-community/embeddinggemma-2-ONNX
Revision: daa72c51243991dfcaf9f9137d2c573d8f7790c0
Graph: onnx/model_quantized.onnx
Precision: q8
Output: sentence_embedding, 768 dimensions
```

The test compares token IDs and vector values. The JSON includes the task prefixes. The Go adapter adds the prefix for the query or document.
