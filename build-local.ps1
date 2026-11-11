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

# Sign the update manifest and archive with the local key when the
# environment does not provide one (CI passes it as a secret). The visible
# copy lives in the user profile; the AppData path is a fallback for older
# builds, where a packaged environment may redirect it.
if (-not $env:MYGO_UPDATER_PRIVATE_KEY) {
    $updateKey = Join-Path $env:USERPROFILE ".xu\update-keys\mygo-update.key"
    if (-not (Test-Path $updateKey)) {
        $updateKey = Join-Path $env:APPDATA "mygo\xu-update-keys\mygo-update.key"
    }
    if (Test-Path $updateKey) {
        $env:MYGO_UPDATER_PRIVATE_KEY = (Get-Content $updateKey -Raw).Trim()
    }
}

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

# Checksums for the published files, in sha256sum's format.
$sumDir = Join-Path $PSScriptRoot "build\windows-amd64"
$releaseFiles = @(
    (Join-Path $PSScriptRoot "build\windows-amd64\Xu.exe"),
    $installer.FullName
)
$updateFiles = Get-ChildItem "$sumDir/update-windows-amd64.json", "$sumDir/xu-*-windows-amd64.tar.gz", "$sumDir/install.ps1" -ErrorAction SilentlyContinue
$releaseFiles += @($updateFiles | ForEach-Object { $_.FullName })
$shaLines = foreach ($f in $releaseFiles) {
    $h = (Get-FileHash -Algorithm SHA256 -LiteralPath $f).Hash.ToLower()
    "$h  $(Split-Path $f -Leaf)"
}
$md5Lines = foreach ($f in $releaseFiles) {
    $h = (Get-FileHash -Algorithm MD5 -LiteralPath $f).Hash.ToLower()
    "$h  $(Split-Path $f -Leaf)"
}
$shaLines | Set-Content (Join-Path $sumDir "SHA256SUMS") -Encoding ascii
$md5Lines | Set-Content (Join-Path $sumDir "MD5SUMS") -Encoding ascii

Write-Host "Build ready:"
Write-Host "  $PSScriptRoot\build\windows-amd64\Xu.exe (portable)"
Write-Host "  $($installer.FullName) (installer)"
Write-Host "  $sumDir\SHA256SUMS"
Write-Host "  $sumDir\MD5SUMS"
if (Test-Path (Join-Path $sumDir "update-windows-amd64.json")) {
    Write-Host "  $sumDir\update-windows-amd64.json (update manifest)"
}
Get-ChildItem (Join-Path $sumDir "xu-*-windows-amd64.tar.gz") -ErrorAction SilentlyContinue | ForEach-Object {
    Write-Host "  $($_.FullName) (update archive)"
}
if (Test-Path (Join-Path $sumDir "install.ps1")) {
    Write-Host "  $sumDir\install.ps1 (update install script)"
}
