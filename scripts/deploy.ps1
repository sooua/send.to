# One-click native deploy for send.to on Windows (PowerShell).
#
# Usage:
#   .\scripts\deploy.ps1                 # build + run in foreground
#   .\scripts\deploy.ps1 -Daemon         # build + run detached
#   .\scripts\deploy.ps1 -Stop           # stop a daemonised instance
#   $env:PORT="9000"; .\scripts\deploy.ps1
#
# Requirements: Go 1.25+, Node 20+, npm on PATH.

[CmdletBinding()]
param(
    [switch]$Daemon,
    [switch]$Stop
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$Port     = if ($env:PORT) { $env:PORT } else { "18080" }
$BuildDir = Join-Path (Get-Location) "build"
$DataDir  = if ($env:DATA_DIR) { $env:DATA_DIR } else { Join-Path (Get-Location) "data" }
$TmpDir   = if ($env:TMP_DIR)  { $env:TMP_DIR  } else { Join-Path $DataDir "tmp" }
$WebDir   = Join-Path (Get-Location) "web\dist"
$PidFile  = Join-Path $BuildDir "sendto.pid"
$LogFile  = Join-Path $BuildDir "sendto.log"
$Binary   = Join-Path $BuildDir "sendto.exe"

function Log($msg)  { Write-Host "[deploy] $msg" -ForegroundColor Cyan }
function Die($msg)  { Write-Host "[deploy] $msg" -ForegroundColor Red; exit 1 }

function Ensure-Tool($name) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        Die "missing required tool: $name"
    }
}

# --- stop mode ---
if ($Stop) {
    if (Test-Path $PidFile) {
        $pid = Get-Content $PidFile
        $proc = Get-Process -Id $pid -ErrorAction SilentlyContinue
        if ($proc) {
            Log "Stopping pid $pid..."
            Stop-Process -Id $pid -Force
        }
        Remove-Item $PidFile -ErrorAction SilentlyContinue
        Log "Stopped."
    } else {
        Log "No PID file at $PidFile — nothing to stop."
    }
    exit 0
}

# --- preflight ---
Ensure-Tool go
Ensure-Tool npm

New-Item -ItemType Directory -Force -Path $BuildDir, $DataDir, $TmpDir | Out-Null

# --- build web ---
Log "Building web bundle..."
Push-Location web
try {
    if (-not (Test-Path node_modules)) {
        npm ci --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) { Die "npm ci failed" }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) { Die "npm run build failed" }
} finally {
    Pop-Location
}

# --- build server ---
Log "Building Go binary ($Binary)..."
$version = (git describe --tags --always --dirty 2>$null)
if (-not $version) { $version = "native" }
$env:CGO_ENABLED = "0"
go build -tags netgo `
    -ldflags "-s -w -X github.com/sooua/send.to/cmd.Version=$version" `
    -trimpath -o $Binary .
if ($LASTEXITCODE -ne 0) { Die "go build failed" }

Log ("Binary: {0:N1} MB  version={1}" -f ((Get-Item $Binary).Length / 1MB), $version)

# --- run ---
$argList = @(
    "--listener", ":$Port",
    "--provider", "local",
    "--basedir", $DataDir,
    "--temp-path", $TmpDir,
    "--web-path", $WebDir,
    "--shutdown-timeout", "30s"
)

if ($Daemon) {
    if ((Test-Path $PidFile) -and (Get-Process -Id (Get-Content $PidFile) -ErrorAction SilentlyContinue)) {
        Die "Already running (pid $(Get-Content $PidFile)). Use -Stop first."
    }
    Log "Starting daemonised on :$Port (logs → $LogFile)..."
    $proc = Start-Process -FilePath $Binary -ArgumentList $argList `
        -RedirectStandardOutput $LogFile -RedirectStandardError $LogFile `
        -WindowStyle Hidden -PassThru
    $proc.Id | Out-File $PidFile
    Start-Sleep -Milliseconds 500
    # Readiness probe
    for ($i = 0; $i -lt 20; $i++) {
        try {
            $r = Invoke-WebRequest "http://127.0.0.1:$Port/health.html" -UseBasicParsing -TimeoutSec 1
            if ($r.StatusCode -eq 200) {
                Log "Running (pid $($proc.Id)). Health: OK  UI: http://127.0.0.1:$Port/"
                exit 0
            }
        } catch { }
        Start-Sleep -Milliseconds 500
    }
    Die "Server did not become ready within 10s. Check $LogFile."
}

Log "Starting in foreground on :$Port (Ctrl+C for graceful shutdown)..."
& $Binary @argList
