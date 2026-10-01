# Find Tools

## Tool Data

The harness makes a search document for a tool. The document has the name, categories, description, input schema, and usage examples when available.

The usage documents for the harness tools are in `internal/tools/search_documents.go`. A plugin can implement `harness.ToolSearchDocumentation`. The MCP server supplies the tool description and schema.

For semantic search, MiniLM makes vectors. The query uses the same model. The harness compares vectors with cosine similarity. A tool's search score is the largest cosine similarity of the document sections.

MiniLM accepts 256 WordPiece tokens for an input. The adapter divides long documents with the model's tokenizer. The sections keep the full document. The end of a schema is not discarded.

Lexical search uses TF-IDF and cosine similarity. It uses the same full documents. Capability descriptions have a weight of 0.75. Full search documents have a weight of 0.25. Lexical search compares words. Semantic search compares model vectors.

The harness keeps vectors in memory. A tool change causes new vectors. A tool removal also removes the vectors from the cache. Lexical search calculates new TF-IDF weights after a tool change.

Search documents, vectors, and search scores are not in model requests or tool results. Conversation vector recall is for a subsequent milestone.

## Tool Result

For example, the model can send:

```json
{"query":"shell command exec"}
```

The result identifies tools:

```json
[{"Name":"bash","Categories":["process","command","shell"]}]
```

The loop gets tool definitions from the registry for the next model request. It gives full descriptions and schemas there.

A full name selects a tool directly. A category without query text gives tools in name sequence. Vectors are not necessary for a full name or category. The default result limit is 10. The maximum is 50.

## Configuration

The `tool_search` section of `providers.json` has the configuration:

```json
{
  "tool_search": {
    "mode": "auto",
    "model_directory": "/opt/mtt/models/all-MiniLM-L6-v2",
    "semantic_minimum_similarity": 0.3,
    "lexical_minimum_similarity": 0.01
  },
  "providers": []
}
```

- `mode`: `auto`, `semantic`, or `lexical`. The default is `auto`.
- `model_directory`: the local model directory. Relative paths start at the directory of the provider file. The field is not necessary in lexical search.
- `semantic_minimum_similarity`: the minimum search score for semantic search. The range is 0 to 1. The default is 0.3.
- `lexical_minimum_similarity`: the minimum search score for lexical search. The range is 0 to 1. The default is 0.01.

In mode `auto`, the harness first selects semantic search. An error during model startup or vector calculation causes automatic fallback to lexical search. The backend stays in lexical search until the harness starts again. The harness records the selected backend and error cause.

In mode `semantic`, an error during model startup stops the harness. An error during vector calculation gives a tool error. It does not give an empty result with status `ok`.

In mode `lexical`, model files and the optional build tag are not necessary. A query without words from the documents gives an empty result.

Cancellation does not cause automatic fallback. It stops subsequent chunks. The chunk in progress completes before model teardown. The harness discards the result. The harness waits for tool discovery before model teardown.

After a configuration change, run:

```sh
docker compose up -d --force-recreate mtt
```

## Model Files

The adapter uses [Hugot v0.7.0](https://github.com/knights-analytics/hugot/tree/v0.7.0). The model runs with Go. CGO and a native library are not necessary.

The image contains [MiniLM](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2) from source revision `1110a243fdf4706b3f48f1d95db1a4f5529b4d41`. The image build gets the ONNX file, tokenizer, and configuration. It compares SHA-256 values. The model license is in the image.

The ONNX file is approximately 90 MB. The model runs in the harness. The harness does not get model assets at runtime. A model server is not necessary.

A text query makes the document cache when the cache is empty. For subsequent queries, the harness calculates vectors only for the query and changed documents.

## Model Adapter

The registry uses `toolsearch.Searcher`. It does not use the model adapter.

The file `cmd/mtt/tool_search_semantic.go` connects the adapter to the harness. The `semantic` build tag includes `cmd/mtt/tool_search_semantic.go` and `internal/molecule/embedding/minilm`.

To include semantic search, run:

```sh
CGO_ENABLED=0 go build -tags semantic ./cmd/mtt
```

For a harness without the model adapter, run:

```sh
CGO_ENABLED=0 go build ./cmd/mtt
docker build --target core -t mtt-harness-core .
```

The core image does not include model assets. The image build for the core image does not get model assets. In mode `auto`, the core image selects lexical search. In mode `semantic`, the core image stops with a startup error.

To remove the optional adapter:

1. Set the mode to `lexical`.
2. Remove `cmd/mtt/tool_search_semantic.go`, `internal/molecule/embedding/minilm`, and `internal/tools/search_semantic_test.go`.
3. Remove `-tags semantic` from commands.
4. Use the Docker target `core`. Remove the optional image stages when they are not necessary.
5. Run `go mod tidy`, `go build ./...`, and `go test ./...`.

The registry, tools, tool results, and lexical backend keep the same interfaces.

## Workspace and Context

The Docker configuration puts the host directory `./workspace` at `/workspace`. Put agent project files in `./workspace`. Configuration files use read-only mounts at `/etc/mtt`.

The loop makes model context in `internal/organism/loop/requests.go`. The function `contextbuilder.Builder.Build` reads session history. A plugin can change messages at the context stage.

The harness reads `start_prompt.md` for system instructions. The template can include session values and tool data from the registry. See `docs/start-prompt.md` for variables and configuration.

## Checks

The container test suite uses the local model. The test database is different from the harness database:

```sh
docker compose --profile test run --build --rm test
```

Run the test suite without the optional adapter:

```sh
CGO_ENABLED=0 go test ./...
```

Run the tool discovery check in the test image:

```sh
docker compose --profile test run --rm test \
  go test -tags semantic -count=1 -v \
  -run TestSemanticDiscoveryWithRealEmbeddings ./internal/tools
```

The test suite examines tool discovery, long schemas, cancellation, automatic fallback, and model teardown. A tool change must cause new vectors. The test suite also examines the provider request after tool discovery.
