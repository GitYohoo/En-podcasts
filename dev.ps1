$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $repoRoot

$env:GOCACHE = Join-Path $repoRoot "build\gocache"
$env:GOTMPDIR = Join-Path $repoRoot "build\gotmp"
$webviewData = Join-Path $repoRoot "build\webview2data"

New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
if (Test-Path $webviewData) {
  Remove-Item -LiteralPath $webviewData -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $webviewData | Out-Null

wails dev
