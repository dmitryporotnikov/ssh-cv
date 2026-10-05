FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/ssh-cv .
RUN mkdir -p /out/data && chown 100:101 /out/data && chmod 700 /out/data

FROM scratch
WORKDIR /app
COPY --from=builder /out/ssh-cv /usr/local/bin/ssh-cv
COPY --from=builder --chown=100:101 /out/data /data
COPY --chown=100:101 config.yaml info.md /app/
ENV SSH_CV_CONFIG_PATH=/app/config.yaml \
    SSH_CV_INFO_PATH=/app/info.md \
    SSH_CV_HOST_KEY_PATH=/data/ssh_host_key \
    GOMAXPROCS=2
VOLUME ["/data"]
EXPOSE 2222
USER 100:101
ENTRYPOINT ["/usr/local/bin/ssh-cv"]
