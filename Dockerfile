FROM golang:1.26-alpine AS builder
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/mtt ./cmd/mtt

FROM alpine:3.22
RUN adduser -D -u 10001 mtt
COPY --from=builder /out/mtt /usr/local/bin/mtt
COPY --from=builder /src/providers.json /src/plugins.json /src/mtt.example.json /workspace/
WORKDIR /workspace
USER mtt
EXPOSE 8080
ENTRYPOINT ["mtt"]
