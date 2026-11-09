# syntax=docker/dockerfile:1

# ---- 构建阶段：交叉编译出静态二进制 ----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ledger .

# ---- 运行阶段 ----
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates \
    && adduser -D -u 1000 ledger
COPY --from=build /out/ledger /usr/local/bin/ledger

USER ledger
VOLUME /data
ENV LEDGER_DATA=/data TZ=Asia/Shanghai
EXPOSE 8300
ENTRYPOINT ["ledger"]
