#Requires -Version 5.1
<#
.SYNOPSIS
    Install pw (project switcher) — PowerShell equivalent of `make install`.
.DESCRIPTION
    Builds pw.exe, copies it and the PowerShell shell helper to ~/go/bin,
    and hooks pw.ps1 into the current user's PowerShell profile.
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
function Write-Step {
    param([string]$Message)
    Write-Host "  --> $Message" -ForegroundColor Cyan
}

function Write-Ok {
    param([string]$Message)
    Write-Host "  [ok] $Message" -ForegroundColor Green
}

function Write-Skip {
    param([string]$Message)
    Write-Host " [skip] $Message" -ForegroundColor DarkGray
}

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------
$RepoRoot   = $PSScriptRoot
$BinDir     = Join-Path $HOME 'go\bin'
$SrcExe     = Join-Path $RepoRoot 'pw.exe'
$SrcPs1     = Join-Path $RepoRoot 'shell\pw.ps1'
$DstExe     = Join-Path $BinDir   'pw.exe'
$DstPs1     = Join-Path $BinDir   'pw.ps1'
$HookLine   = '. "$HOME\go\bin\pw.ps1"'
$HookComment = '# pw project switcher'

# ---------------------------------------------------------------------------
# Step 1 — Build
# ---------------------------------------------------------------------------
Write-Step 'Building pw.exe ...'
try {
    Push-Location $RepoRoot
    & go build -o pw.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build exited with code $LASTEXITCODE" }
    Write-Ok 'Built pw.exe'
} finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# Step 2 — Ensure ~/go/bin exists
# ---------------------------------------------------------------------------
if (-not (Test-Path $BinDir)) {
    Write-Step "Creating $BinDir ..."
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    Write-Ok "Created $BinDir"
} else {
    Write-Skip "$BinDir already exists"
}

# ---------------------------------------------------------------------------
# Step 3 — Copy pw.exe (skip if already up to date)
# ---------------------------------------------------------------------------
$needCopyExe = $true
if (Test-Path $DstExe) {
    $srcHash = (Get-FileHash $SrcExe -Algorithm SHA256).Hash
    $dstHash = (Get-FileHash $DstExe -Algorithm SHA256).Hash
    if ($srcHash -eq $dstHash) {
        $needCopyExe = $false
        Write-Skip "pw.exe already up to date in $BinDir"
    }
}
if ($needCopyExe) {
    Write-Step "Copying pw.exe to $BinDir ..."
    Copy-Item -Path $SrcExe -Destination $DstExe -Force
    Write-Ok "Installed pw.exe -> $DstExe"
}

# ---------------------------------------------------------------------------
# Step 4 — Copy shell/pw.ps1
# ---------------------------------------------------------------------------
if (-not (Test-Path $SrcPs1)) {
    Write-Host "[warn] $SrcPs1 not found — skipping shell helper copy." -ForegroundColor Yellow
} else {
    $needCopyPs1 = $true
    if (Test-Path $DstPs1) {
        $srcHash = (Get-FileHash $SrcPs1 -Algorithm SHA256).Hash
        $dstHash = (Get-FileHash $DstPs1 -Algorithm SHA256).Hash
        if ($srcHash -eq $dstHash) {
            $needCopyPs1 = $false
            Write-Skip "pw.ps1 already up to date in $BinDir"
        }
    }
    if ($needCopyPs1) {
        Write-Step "Copying shell\pw.ps1 to $BinDir ..."
        Copy-Item -Path $SrcPs1 -Destination $DstPs1 -Force
        Write-Ok "Installed pw.ps1  -> $DstPs1"
    }
}

# ---------------------------------------------------------------------------
# Step 5 — Hook pw.ps1 into PowerShell profile
# ---------------------------------------------------------------------------
Write-Step "Checking PowerShell profile ($PROFILE) ..."

# Ensure profile directory exists
$profileDir = Split-Path $PROFILE -Parent
if (-not (Test-Path $profileDir)) {
    New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
    Write-Ok "Created profile directory: $profileDir"
}

# Ensure profile file exists
if (-not (Test-Path $PROFILE)) {
    New-Item -ItemType File -Path $PROFILE -Force | Out-Null
    Write-Ok "Created profile file: $PROFILE"
}

$profileContent = Get-Content -Path $PROFILE -Raw -ErrorAction SilentlyContinue
if ($null -eq $profileContent) { $profileContent = '' }

if ($profileContent -like "*$HookLine*") {
    Write-Skip "pw hook already present in profile"
} else {
    # Append with a blank line separator, comment, and the dot-source line
    $append = "`n$HookComment`n$HookLine`n"
    Add-Content -Path $PROFILE -Value $append -NoNewline:$false
    Write-Ok "Added pw hook to $PROFILE"
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host 'pw installed successfully.' -ForegroundColor Green
Write-Host "Restart your PowerShell session (or run: . `$PROFILE) to activate." -ForegroundColor Yellow
