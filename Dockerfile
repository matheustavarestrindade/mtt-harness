FROM alpine:3.22 AS model-assets
RUN apk add --no-cache ca-certificates
WORKDIR /opt/mtt/models/all-MiniLM-L6-v2
# Pin the official export and tokenizer together. Download at build time only;
# runtime initialization and searches need no network or extra model service.
ARG MODEL_REVISION=1110a243fdf4706b3f48f1d95db1a4f5529b4d41
RUN wget -q -O model.onnx "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/${MODEL_REVISION}/onnx/model.onnx" \
    && wget -q -O tokenizer.json "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/${MODEL_REVISION}/tokenizer.json" \
    && wget -q -O config.json "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/${MODEL_REVISION}/config.json" \
    && wget -q -O sentence_bert_config.json "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/${MODEL_REVISION}/sentence_bert_config.json" \
    && wget -q -O LICENSE "https://www.apache.org/licenses/LICENSE-2.0.txt" \
    && echo "6fd5d72fe4589f189f8ebc006442dbb529bb7ce38f8082112682524616046452  model.onnx" | sha256sum -c - \
    && echo "be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037  tokenizer.json" | sha256sum -c - \
    && echo "953f9c0d463486b10a6871cc2fd59f223b2c70184f49815e7efbcab5d8908b41  config.json" | sha256sum -c - \
    && echo "fc1993fde0a95c24ec6c022539d41cf6e2f7c9721e5415d6fb6897472a9cd4b7  sentence_bert_config.json" | sha256sum -c -

FROM golang:1.26-alpine AS core-builder
RUN apk add --no-cache git
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/mtt ./cmd/mtt

FROM python:3.13-slim-bookworm AS context-assets
ARG TARGETARCH
COPY scripts/download-context-model.py scripts/download-embedding-runtime.py /build/
RUN python /build/download-context-model.py /opt/mtt/models/embeddinggemma-2
RUN python /build/download-embedding-runtime.py "${TARGETARCH}" /opt/mtt/lib

FROM golang:1.26-bookworm AS builder
ENV CGO_ENABLED=1 CGO_LDFLAGS=-L/opt/mtt/lib
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY --from=model-assets /opt/mtt/models /opt/mtt/models
COPY --from=context-assets /opt/mtt /opt/mtt
COPY . .
RUN go build -tags semantic,gemma -trimpath -ldflags="-s -w" -o /out/mtt ./cmd/mtt

FROM alpine:3.22 AS core
RUN adduser -D -u 10001 mtt
COPY --from=core-builder /out/mtt /usr/local/bin/mtt
COPY --from=core-builder /src/providers.json /src/mcp.example.json /src/mtt.example.json /src/start_prompt.md /etc/mtt/
COPY --from=core-builder /src/plugins/spaced_repetition/prompts /etc/mtt/spaced-repetition
COPY --from=core-builder /src/plugins/sidekick/prompt.md /etc/mtt/sidekick.md
WORKDIR /workspace
USER mtt
EXPOSE 8080
ENTRYPOINT ["mtt"]
CMD ["--start-prompt-file", "/etc/mtt/start_prompt.md", "--spaced-repetition-prompts", "/etc/mtt/spaced-repetition", "--sidekick-prompt-file", "/etc/mtt/sidekick.md"]

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget libstdc++6 libgomp1 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 mtt
COPY --from=builder /out/mtt /usr/local/bin/mtt
COPY --from=model-assets /opt/mtt/models /opt/mtt/models
COPY --from=context-assets /opt/mtt/models /opt/mtt/models
COPY --from=context-assets /opt/mtt/lib/libonnxruntime.so.1.30.0 /opt/mtt/lib/onnxruntime-LICENSE /opt/mtt/lib/onnxruntime-ThirdPartyNotices.txt /opt/mtt/lib/
COPY --from=builder /src/providers.json /src/mcp.example.json /src/mtt.example.json /src/start_prompt.md /etc/mtt/
COPY --from=builder /src/plugins/spaced_repetition/prompts /etc/mtt/spaced-repetition
COPY --from=builder /src/plugins/sidekick/prompt.md /etc/mtt/sidekick.md
WORKDIR /workspace
USER mtt
EXPOSE 8080
ENTRYPOINT ["mtt"]
CMD ["--start-prompt-file", "/etc/mtt/start_prompt.md", "--spaced-repetition-prompts", "/etc/mtt/spaced-repetition", "--sidekick-prompt-file", "/etc/mtt/sidekick.md"]
