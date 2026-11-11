# Build the portable Windows executable with its icon and native DLLs
# embedded, plus the NSIS installer built from it. Outputs:
#   build\windows-amd64\Xu.exe
#   build\windows-amd64\<slug>-setup-<version>-amd64.exe
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

# Second pass: embed those DLLs into the executable and build the NSIS
# installer from the resulting files. Without -skip-installer mygo downloads
# NSIS when it is not installed.
$oldGoFlags = $env:GOFLAGS
try {
    $env:GOFLAGS = "-tags=portable"
    go tool mygo build -platform windows/amd64
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

$installer = Get-ChildItem "build/windows-amd64/*-setup-*-amd64.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $installer) {
    throw "NSIS installer was not created in build\windows-amd64"
}

# Checksums for the two published files, in sha256sum's format.
$releaseFiles = @(
    (Join-Path $PSScriptRoot "build\windows-amd64\Xu.exe"),
    $installer.FullName
)
$shaLines = foreach ($f in $releaseFiles) {
    $h = (Get-FileHash -Algorithm SHA256 -LiteralPath $f).Hash.ToLower()
    "$h  $(Split-Path $f -Leaf)"
}
$md5Lines = foreach ($f in $releaseFiles) {
    $h = (Get-FileHash -Algorithm MD5 -LiteralPath $f).Hash.ToLower()
    "$h  $(Split-Path $f -Leaf)"
}
$sumDir = Join-Path $PSScriptRoot "build\windows-amd64"
$shaLines | Set-Content (Join-Path $sumDir "SHA256SUMS") -Encoding ascii
$md5Lines | Set-Content (Join-Path $sumDir "MD5SUMS") -Encoding ascii

Write-Host "Build ready:"
Write-Host "  $PSScriptRoot\build\windows-amd64\Xu.exe (portable)"
Write-Host "  $($installer.FullName) (installer)"
Write-Host "  $sumDir\SHA256SUMS"
Write-Host "  $sumDir\MD5SUMS"
