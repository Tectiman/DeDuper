# Windows PowerShell build script for current platform
$ErrorActionPreference = 'Stop'

# 清理旧的构建产物
if (Test-Path "dist") {
    Remove-Item -Recurse -Force "dist"
}
New-Item -ItemType Directory -Path "dist" | Out-Null

Write-Host "开始构建当前平台二进制文件..."

# 获取当前平台信息
$os = [System.Runtime.InteropServices.RuntimeInformation]::OSPlatform.ToString()
$arch = [System.Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString()

# 转换操作系统名称
switch ($os) {
    "Windows" { $GOOS = "windows" }
    "Linux" { $GOOS = "linux" }
    "OSX" { $GOOS = "darwin" }
    "FreeBSD" { $GOOS = "freebsd" }
    default {
        Write-Host "未知操作系统：$os"
        exit 1
    }
}

# 转换架构名称
switch ($arch) {
    "X64" { $GOARCH = "amd64" }
    "Arm64" { $GOARCH = "arm64" }
    "X86" { $GOARCH = "386" }
    "Arm" { 
        $GOARCH = "arm"
        $GOARM = "7"
    }
    default {
        Write-Host "未知架构：$arch"
        exit 1
    }
}

# 确定输出文件名
if ($GOOS -eq "windows") {
    $OUTPUT = "dist\dedup-${GOOS}-${GOARCH}.exe"
} else {
    $OUTPUT = "dist\dedup-${GOOS}-${GOARCH}"
}

Write-Host "构建 ${GOOS}/${GOARCH} -> ${OUTPUT}"

# 构建优化选项:
# -trimpath: 移除文件路径，提高可重现性
# -ldflags="-s -w": 去除符号表和调试信息，减小二进制大小
$env:GOOS = $GOOS
$env:GOARCH = $GOARCH
if ($GOARM) {
    $env:GOARM = $GOARM
}

go build -trimpath -ldflags="-s -w" -o $OUTPUT main.go

# 清除环境变量
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
if ($GOARM) {
    Remove-Item Env:\GOARM -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "构建完成！二进制文件位于 dist/ 目录:"
Get-ChildItem dist/ | Select-Object Name, Length