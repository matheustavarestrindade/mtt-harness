# Memory Embeddings

## Model

The standard harness image uses EmbeddingGemma 2 for workspace memory. The native adapter runs in the harness process with ONNX Runtime and Hugging Face Tokenizers. A model server is not necessary. Tool discovery keeps the MiniLM adapter.

The native adapter uses text input only. The image build does not get image or audio model files. Image, audio, and `video` content keeps the text description from the conversation. Different input types are subsequent work.

```text
Base model: google/embeddinggemma-2
Export: onnx-community/embeddinggemma-2-ONNX
Revision: daa72c51243991dfcaf9f9137d2c573d8f7790c0
Precision: Q8
Text parameters: 270 million
Vector dimensions: 768
Maximum input: 8192 tokens
Default text chunk: 1024 tokens
Default CPU threads: 2
ONNX Runtime: 1.30.0
Go binding: github.com/yalue/onnxruntime_go v1.27.0
Tokenizer binding: github.com/daulet/tokenizers v1.26.0
License for model assets: Apache 2.0
```

The image build gets model assets and native libraries and compares SHA-256 values. The model runs without network access. The native adapter sets ONNX Runtime telemetry to `OFF`.

The model assets have approximately 346 MB of data. The number does not give runtime memory usage.

## Configuration

The configuration for memory embeddings is in `providers.json`. The `context_embeddings` object gives model assets and CPU settings. The database keeps plugin behavior and worker model settings. Provider credentials and prices do not change.

```json
{
  "context_embeddings": {
    "backend": "embeddinggemma2",
    "model_directory": "/opt/mtt/models/embeddinggemma-2",
    "model_file": "onnx/model_quantized.onnx",
    "runtime_library": "/opt/mtt/lib/libonnxruntime.so.1.30.0",
    "chunk_tokens": 1024,
    "threads": 2
  }
}
```

The backends are `embeddinggemma2`, `minilm`, and `disabled`. The default is `disabled` when the configuration object is not in the file. Automatic fallback between memory models is not available. Vectors from different models do not have the same vector space.

Relative paths start at the provider file directory. The model file path starts at the model directory. The native library path is necessary for `embeddinggemma2`. For `minilm`, the model directory and `semantic` build tag are necessary.

The text chunk range is 32 to 8192 tokens. The range for CPU threads is 1 to 64. Start the harness again after a configuration change.

```text
chunk_tokens: zero or missing -> 1024 tokens
threads: zero or missing -> 2 CPU threads
```

## Text and Queries

Documents and queries use different prefixes:

```text
Document: title: none | text: 
Query: task: search result | query: 
```

The tokenizer adds BOS and EOS tokens. The text chunk limit includes special tokens and the larger prefix. The native adapter keeps source text bytes and line breaks. Input text that is not UTF-8 gives an error. A NUL byte also gives an error. The native adapter does not cut text after the token limit.

Text that contains a media marker, such as `<|image|>`, stays text. It does not start a model encoder for a different input type. The native adapter gives empty feature tensors to the model graph for image, `video`, and audio data.

The `remember` tool writes input text and embeddings before it gives `saved`. It does not change the input text or start a model request. Query vectors use the optional `harness.QueryTextEmbedder` interface. A model encoder can use `TextEmbedder` when the query and document formats are the same.

## Cancellation

One model encoder controls the tokenizer and native inference. The model encoder uses one text chunk at a time. Context cancellation stops active native inference. Cancellation also stops a request that waits for the model encoder.

The native adapter first stops native inference. It then releases native resources. Tokenizer work completes before the tokenizer can close. The native library stays open during model startup. A request does not start native inference after cancellation.

The harness waits for memory workers before it closes the model encoder and native environment.

## Image and Checks

The standard harness image uses Debian Bookworm and CGO. The database image keeps Postgres 16 Alpine and pgvector. CGO is not necessary for the `core` image. The core image does not get model assets.

```sh
docker compose up --build
docker compose --profile test run --build --rm test
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
```

The `gemma` build tag includes the native adapter. The `semantic` build tag includes MiniLM for tool discovery. The native libraries are necessary for the command:

```sh
CGO_ENABLED=1 CGO_LDFLAGS=-L/path/to/native/libraries \
  go build -tags semantic,gemma ./cmd/mtt
```

The test suite compares token IDs and vectors with output from Transformers.js 4.3.1 and ONNX Runtime 1.30.0. The reference vectors are in `internal/molecule/embedding/embeddinggemma/testdata`.

The test suite also examines text chunk limits, cancellation, native resources, and memory search. It examines workspace boundaries.

## Replacement and Removal

The model identity includes model files, tokenizer files, prefixes, vector dimensions, and text chunk settings. After a model identity change, the harness must not use the previous vector index. The harness does not remove memory records after a model change.

To remove native inference, set the backend to `disabled` or `minilm`. Use a Go command without `-tags gemma`. Remove native libraries from the image. The native adapter code is in `internal/molecule/embedding/embeddinggemma`.

The program import is in `cmd/mtt/context_embeddings_gemma.go`. The file `cmd/mtt/context_embeddings_gemma_disabled.go` gives an error when the native adapter is not available.

Keep memory records and source data during adapter removal. Stop memory workers before you remove data. Keep provider credentials, workspace settings, and other harness data.
