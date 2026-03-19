#!/bin/bash
set -e

# 清理旧的构建产物
rm -rf dist/
mkdir -p dist/

echo "开始构建当前平台二进制文件..."

# 获取当前平台
CURRENT_OS=$(uname -s)
CURRENT_ARCH=$(uname -m)

# 转换操作系统名称
case "$CURRENT_OS" in
    Linux)
        GOOS="linux"
        ;;
    Darwin)
        GOOS="darwin"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        GOOS="windows"
        ;;
    FreeBSD)
        GOOS="freebsd"
        ;;
    OpenBSD)
        GOOS="openbsd"
        ;;
    NetBSD)
        GOOS="netbsd"
        ;;
    *)
        echo "未知操作系统：$CURRENT_OS"
        exit 1
        ;;
esac

# 转换架构名称
case "$CURRENT_ARCH" in
    x86_64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    armv7l)
        GOARCH="arm"
        GOARM="7"
        ;;
    *)
        echo "未知架构：$CURRENT_ARCH"
        exit 1
        ;;
esac

# 确定输出文件名
if [ "$GOOS" = "windows" ]; then
    OUTPUT="dist/dedup-${GOOS}-${GOARCH}.exe"
else
    OUTPUT="dist/dedup-${GOOS}-${GOARCH}"
fi

echo "构建 ${GOOS}/${GOARCH} -> ${OUTPUT}"

# 构建优化选项:
# -trimpath: 移除文件路径，提高可重现性
# -ldflags="-s -w": 去除符号表和调试信息，减小二进制大小
# -pgo=default: 启用性能分析引导优化 (Go 1.20+)
LDFLAGS="-s -w"

if [ -n "$GOARM" ]; then
    GOOS=$GOOS GOARCH=$GOARCH GOARM=$GOARM go build -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT" main.go
else
    GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags="$LDFLAGS" -o "$OUTPUT" main.go
fi