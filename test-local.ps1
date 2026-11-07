# Run tests with Go's temporary files inside the project, so antivirus
# software that trusts this folder does not quarantine *.test.exe files.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$env:GOTMPDIR = Join-Path $PSScriptRoot ".gotmp"
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null

go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

go vet ./...
exit $LASTEXITCODE
