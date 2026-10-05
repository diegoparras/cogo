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
# CA roots: scratch has none, so any outbound HTTPS (Lockatus OIDC discovery/JWKS,
# a remote LLM, Anonimal) would fail cert verification. Copy the bundle in.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /cogo /cogo
ENV COGO_VAULT=/vault
EXPOSE 8080
VOLUME ["/vault"]
# En scratch no hay curl ni shell: el binario se prueba a sí mismo. `cogo health`
# pega a /healthz, que verifica que el vault se lee y el registro responde.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/cogo", "health"]
# Default: long-running MCP-over-HTTP service (the suite/compose use case).
# For a one-shot local stdio server, override:  docker run -i cogo serve
ENTRYPOINT ["/cogo"]
CMD ["serve", "-http", ":8080"]
