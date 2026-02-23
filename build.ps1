# Windows PowerShell build script for cross-platform builds
$ErrorActionPreference = 'Stop'

# 清理旧的构建产物
if (Test-Path "dist") {
    Remove-Item -Recurse -Force "dist"
}
New-Item -ItemType Directory -Path "dist" | Out-Null

Write-Host "开始构建跨平台二进制文件..."

# 定义目标平台和架构
$platforms = @(
    @{GOOS="linux"; GOARCH="amd64"},
    @{GOOS="linux"; GOARCH="arm64"},
    @{GOOS="darwin"; GOARCH="amd64"},
    @{GOOS="darwin"; GOARCH="arm64"},
    @{GOOS="windows"; GOARCH="amd64"},
    @{GOOS="windows"; GOARCH="arm64"},
    @{GOOS="freebsd"; GOARCH="amd64"},
    @{GOOS="freebsd"; GOARCH="arm64"},
    @{GOOS="openbsd"; GOARCH="amd64"},
    @{GOOS="openbsd"; GOARCH="arm64"},
    @{GOOS="netbsd"; GOARCH="amd64"},
    @{GOOS="netbsd"; GOARCH="arm64"}
)

# 构建每个平台
foreach ($platform in $platforms) {
    $GOOS = $platform.GOOS
    $GOARCH = $platform.GOARCH
    
    # 确定输出文件名
    if ($GOOS -eq "windows") {
        $OUTPUT = "dist\dedup-${GOOS}-${GOARCH}.exe"
    } else {
        $OUTPUT = "dist\dedup-${GOOS}-${GOARCH}"
    }
    
    Write-Host "构建 ${GOOS}/${GOARCH} -> ${OUTPUT}"
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
    go build -o $OUTPUT main.go
}

# 清除环境变量
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "构建完成！二进制文件位于 dist/ 目录:"
Get-ChildItem dist/ | Select-Object Name, Length

Write-Host ""
Write-Host "平台说明:"
Write-Host "  - linux:   Linux (Ubuntu, Debian, CentOS, etc.)"
Write-Host "  - darwin:  macOS"
Write-Host "  - windows: Windows"
Write-Host "  - freebsd: FreeBSD"
Write-Host "  - openbsd: OpenBSD"
Write-Host "  - netbsd:  NetBSD"

# 如果只需要 Windows 版本，使用以下命令:
# Write-Host ""
# Write-Host "仅构建 Windows 版本:"
# go build -o dedup.exe main.go
# Write-Host "Build complete: dedup.exe"