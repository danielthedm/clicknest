FROM node:20-slim AS sdk-builder
WORKDIR /build/sdk
COPY sdk/package.json sdk/package-lock.json* ./
RUN npm ci
COPY sdk/ ./
RUN npm run build

FROM node:20-slim AS web-builder
WORKDIR /build/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=sdk-builder /build/sdk/dist ./cmd/clicknest/sdk_dist
COPY --from=web-builder /build/web/build ./cmd/clicknest/web_build
RUN CGO_ENABLED=1 go build -o /clicknest ./cmd/clicknest/

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /clicknest /usr/local/bin/clicknest
RUN mkdir -p /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["clicknest"]
CMD ["-addr", ":8080", "-data", "/data"]
