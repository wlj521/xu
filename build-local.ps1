# Build a single portable Windows executable with its icon and native DLLs
# embedded. The output is build\windows-amd64\Xu.exe only.
# Keep this folder in your antivirus trust list: unsigned Go builds often
# trip heuristics in 360 and similar scanners.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

# Keep Go's temporary build files inside the trusted project folder.
$env:GOTMPDIR = Join-Path $PSScriptRoot ".gotmp"
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null

Get-Process Xu -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300

# First pass: produce the two native DLLs in build\windows-amd64.
$env:GOFLAGS = ""
go tool mygo build -platform windows/amd64 -skip-installer
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$assets = Join-Path $PSScriptRoot "internal\bundle\assets"
New-Item -ItemType Directory -Force $assets | Out-Null
Copy-Item "build/windows-amd64/*.dll" $assets -Force

# Second pass: embed those DLLs into the executable. The DLLs are removed from
# the source tree after compiling; only the single exe remains in build.
$oldGoFlags = $env:GOFLAGS
try {
    $env:GOFLAGS = "-tags=portable"
    go tool mygo build -platform windows/amd64 -skip-installer
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    if ($null -eq $oldGoFlags) {
        Remove-Item Env:GOFLAGS -ErrorAction SilentlyContinue
    } else {
        $env:GOFLAGS = $oldGoFlags
    }
    Remove-Item (Join-Path $assets "*.dll") -Force -ErrorAction SilentlyContinue
}

# The DLLs are now inside Xu.exe, and mygo already embedded icon.png as the
# executable's icon. Remove the sidecar copies so the output is one file.
Remove-Item "build/windows-amd64/*.dll" -Force -ErrorAction SilentlyContinue
Remove-Item "build/windows-amd64/icon.png" -Force -ErrorAction SilentlyContinue

Write-Host "Build ready: $PSScriptRoot\build\windows-amd64\Xu.exe"
