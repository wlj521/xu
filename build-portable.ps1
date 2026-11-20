# Build a clean, single-file Windows portable package.
# Outputs:
#   build\Xu.exe
#   build\Xu-<version>-portable-windows-amd64.zip
#   build\SHA256SUMS
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$root = [System.IO.Path]::GetFullPath($PSScriptRoot)
$buildDir = [System.IO.Path]::GetFullPath((Join-Path $root "build"))
$rootPrefix = $root.TrimEnd("\") + "\"
if (-not $buildDir.StartsWith($rootPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing to clean a build directory outside the project: $buildDir"
}

$buildPrefix = $buildDir.TrimEnd("\") + "\"
$runningFromBuild = @(Get-Process Xu -ErrorAction SilentlyContinue | Where-Object {
    $_.Path -and [System.IO.Path]::GetFullPath($_.Path).StartsWith($buildPrefix, [System.StringComparison]::OrdinalIgnoreCase)
})
if ($runningFromBuild.Count -gt 0) {
    throw "检测到 Xu 正在从 build 目录运行。请先手动退出它再重新打包；本脚本不会结束任何 Xu 进程。"
}

if (Test-Path -LiteralPath $buildDir) {
    Remove-Item -LiteralPath $buildDir -Recurse -Force
}

$env:GOTMPDIR = Join-Path $root ".gotmp"
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null

$assets = Join-Path $root "internal\bundle\assets"
New-Item -ItemType Directory -Force $assets | Out-Null
try {
    $env:GOFLAGS = ""
    go tool mygo build -platform windows/amd64 -skip-installer
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Copy-Item (Join-Path $buildDir "windows-amd64\*.dll") $assets -Force
    $env:GOFLAGS = "-tags=portable"
    go tool mygo build -platform windows/amd64 -skip-installer
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Remove-Item (Join-Path $assets "*.dll") -Force -ErrorAction SilentlyContinue
    Remove-Item Env:GOFLAGS -ErrorAction SilentlyContinue
}

$exe = Join-Path $buildDir "Xu.exe"
Copy-Item (Join-Path $buildDir "windows-amd64\Xu.exe") $exe -Force
Remove-Item (Join-Path $buildDir "windows-amd64") -Recurse -Force

$version = (Get-Content (Join-Path $root "mygo.json") -Raw | ConvertFrom-Json).version
$zip = Join-Path $buildDir "Xu-$version-portable-windows-amd64.zip"
Compress-Archive -LiteralPath $exe -DestinationPath $zip -Force

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $exe).Hash.ToLower()
"$hash  Xu.exe" | Set-Content (Join-Path $buildDir "SHA256SUMS") -Encoding ascii

Write-Host "Portable build ready:"
Write-Host "  $exe"
Write-Host "  $zip"
Write-Host "  $(Join-Path $buildDir 'SHA256SUMS')"
