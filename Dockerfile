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

# ---- 运行阶段：scratch，零运行命令（无需 QEMU 即可跨架构构建） ----
# 时区表已通过 time/tzdata 内嵌进二进制；应用无出站 TLS 调用，不需要 CA 证书
FROM scratch
COPY --from=build /out/ledger /ledger

# 65534 = nobody（scratch 里没有 useradd，直接用 UID）
# 数据目录的属主需在宿主机上 chown 65534:65534
USER 65534:65534
VOLUME /data
ENV LEDGER_DATA=/data TZ=Asia/Shanghai
EXPOSE 8300
ENTRYPOINT ["/ledger"]
