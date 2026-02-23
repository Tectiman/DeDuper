#!/bin/bash
set -e

# 清理旧的构建产物
rm -rf dist/
mkdir -p dist/

echo "开始构建跨平台二进制文件..."

# 定义目标平台和架构
# 格式：GOOS/GOARCH
platforms=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
    "freebsd/amd64"
    "freebsd/arm64"
    "openbsd/amd64"
    "openbsd/arm64"
    "netbsd/amd64"
    "netbsd/arm64"
)

# 构建每个平台
for platform in "${platforms[@]}"; do
    GOOS=$(echo $platform | cut -d'/' -f1)
    GOARCH=$(echo $platform | cut -d'/' -f2)
    
    # 确定输出文件名
    if [ "$GOOS" = "windows" ]; then
        OUTPUT="dist/dedup-${GOOS}-${GOARCH}.exe"
    else
        OUTPUT="dist/dedup-${GOOS}-${GOARCH}"
    fi
    
    echo "构建 ${GOOS}/${GOARCH} -> ${OUTPUT}"
    GOOS=$GOOS GOARCH=$GOARCH go build -o "$OUTPUT" main.go
done

echo ""
echo "构建完成！二进制文件位于 dist/ 目录:"
ls -la dist/
echo ""
echo "平台说明:"
echo "  - linux:   Linux (Ubuntu, Debian, CentOS, etc.)"
echo "  - darwin:  macOS"
echo "  - windows: Windows"
echo "  - freebsd: FreeBSD"
echo "  - openbsd: OpenBSD"
echo "  - netbsd:  NetBSD"