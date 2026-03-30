FROM golang:1.24-alpine AS builder

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/ssh-cv .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
	&& addgroup -S app \
	&& adduser -S -G app -h /app app \
	&& mkdir -p /app /data \
	&& chown -R app:app /app /data

WORKDIR /app

COPY --from=builder /out/ssh-cv /usr/local/bin/ssh-cv
COPY --chown=app:app config.yaml info.md /app/

ENV SSH_CV_CONFIG_PATH=/app/config.yaml
ENV SSH_CV_INFO_PATH=/app/info.md
ENV SSH_CV_HOST_KEY_PATH=/data/ssh_host_key

VOLUME ["/data"]

EXPOSE 2222

USER app

ENTRYPOINT ["/usr/local/bin/ssh-cv"]
