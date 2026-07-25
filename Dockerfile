# Multi-stage: static Go binary → tiny runtime image
# Anyone can: podman run -it --rm --network=host … agenterm
FROM docker.io/library/golang:1.25-alpine AS build
WORKDIR /src

ARG VERSION=0.3.0

RUN apk add --no-cache git ca-certificates
ENV GOTOOLCHAIN=local CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy \
    && go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/agenterm ./cmd/agenterm

FROM docker.io/library/alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /home/agenterm agenterm

USER agenterm
WORKDIR /home/agenterm
COPY --from=build /out/agenterm /usr/local/bin/agenterm

# Default: talk to Ollama on the host (use --network=host or host.containers.internal)
ENV AGENTERM_PROVIDER=ollama-local \
    AGENTERM_BASE_URL=http://127.0.0.1:11434/v1 \
    AGENTERM_MODEL=qwen2.5-coder:32b \
    AGENTERM_API_KEY=ollama \
    TERM=xterm-256color \
    COLORTERM=truecolor

# Config lives here when you bind-mount ~/.agenterm
VOLUME ["/home/agenterm/.agenterm"]

ENTRYPOINT ["agenterm"]
# default: interactive TUI; override e.g. --ping
CMD []
