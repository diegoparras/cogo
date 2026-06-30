# syntax=docker/dockerfile:1

# ---- build: a single static binary, no CGO ----
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -ldflags="-s -w" -o /cogo ./cmd/cogo

# ---- runtime: scratch — just the binary, a few MB ----
FROM scratch
COPY --from=build /cogo /cogo
ENV COGO_VAULT=/vault
EXPOSE 8080
VOLUME ["/vault"]
# Default: long-running MCP-over-HTTP service (the suite/compose use case).
# For a one-shot local stdio server, override:  docker run -i cogo serve
ENTRYPOINT ["/cogo"]
CMD ["serve", "-http", ":8080"]
