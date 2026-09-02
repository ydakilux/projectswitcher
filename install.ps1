#Requires -Version 5.1
<#
.SYNOPSIS
    Install pw (project switcher) - PowerShell equivalent of `make install`.
.DESCRIPTION
    Builds pw.exe, copies it and the PowerShell shell helper to ~/go/bin,
    and hooks pw-profile.ps1 into the current user's PowerShell profile.
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

# Get-FileHash requires the Microsoft.PowerShell.Utility module to be
# auto-loaded, which can fail in some restricted/managed environments
# (e.g. a stripped-down PSModulePath in the process that spawned this
# script). Direct .NET calls don't depend on module auto-loading, so use
# those instead for a reliable file-content comparison.
function Get-Sha256Hex {
    param([string]$Path)
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hashBytes = $sha.ComputeHash($bytes)
        return [System.BitConverter]::ToString($hashBytes) -replace '-', ''
    } finally {
        $sha.Dispose()
    }
}

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------
$RepoRoot   = $PSScriptRoot
$BinDir     = Join-Path $HOME 'go\bin'
$SrcExe     = Join-Path $RepoRoot 'pw.exe'
$SrcPs1     = Join-Path $RepoRoot 'shell\pw-profile.ps1'
$DstExe     = Join-Path $BinDir   'pw.exe'
$DstPs1     = Join-Path $BinDir   'pw-profile.ps1'
$HookLine    = '. "$HOME\go\bin\pw-profile.ps1"'
$HookComment = '# pw project switcher'
# Bundled with the hook so a default/Restricted execution policy doesn't
# block loading the (unsigned) pw-profile.ps1 script. Scoped to Process only
# - it doesn't persist anywhere and doesn't require writing to any config
# file, so it can't be blocked the same way profile writes sometimes are.
$PolicyLine = 'Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force'
# Regex matching any stale hook line from before the pw.ps1 -> pw-profile.ps1
# rename, so old profiles get cleaned up instead of erroring on load.
$StaleHookPattern = '^\s*\.\s+"?\$HOME\\go\\bin\\pw\.ps1"?\s*$'

# ---------------------------------------------------------------------------
# Step 1 - Build
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
# Step 2 - Ensure ~/go/bin exists
# ---------------------------------------------------------------------------
if (-not (Test-Path $BinDir)) {
    Write-Step "Creating $BinDir ..."
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    Write-Ok "Created $BinDir"
} else {
    Write-Skip "$BinDir already exists"
}

# ---------------------------------------------------------------------------
# Step 3 - Copy pw.exe (skip if already up to date)
# ---------------------------------------------------------------------------
$needCopyExe = $true
if (Test-Path $DstExe) {
    $srcHash = (Get-Sha256Hex $SrcExe)
    $dstHash = (Get-Sha256Hex $DstExe)
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
# Step 4 - Copy shell/pw-profile.ps1
# ---------------------------------------------------------------------------
if (-not (Test-Path $SrcPs1)) {
    Write-Host "[warn] $SrcPs1 not found - skipping shell helper copy." -ForegroundColor Yellow
} else {
    $needCopyPs1 = $true
    if (Test-Path $DstPs1) {
        $srcHash = (Get-Sha256Hex $SrcPs1)
        $dstHash = (Get-Sha256Hex $DstPs1)
        if ($srcHash -eq $dstHash) {
            $needCopyPs1 = $false
            Write-Skip "pw-profile.ps1 already up to date in $BinDir"
        }
    }
    if ($needCopyPs1) {
        Write-Step "Copying shell\pw-profile.ps1 to $BinDir ..."
        Copy-Item -Path $SrcPs1 -Destination $DstPs1 -Force
        Write-Ok "Installed pw-profile.ps1 -> $DstPs1"
    }
}

# Remove stale pw.ps1 from a previous install (renamed to pw-profile.ps1).
# A leftover pw.ps1 next to pw.exe in BinDir would shadow the `pw` command
# (PowerShell resolves bare script/exe names, and pw.ps1 isn't signed).
$staleOldPs1 = Join-Path $BinDir 'pw.ps1'
if (Test-Path $staleOldPs1) {
    Remove-Item -Path $staleOldPs1 -Force
    Write-Ok "Removed stale $staleOldPs1 from a previous install"
}

# ---------------------------------------------------------------------------
# Step 5 - Hook pw-profile.ps1 into PowerShell profiles
# ---------------------------------------------------------------------------
# Ctrl+T can open a new tab in either Windows PowerShell 5.1 or PowerShell 7
# (best-effort guess), so hook both editions' profiles, not just the one
# running this installer. Both live under the same Documents folder, in
# WindowsPowerShell\ and PowerShell\ respectively.
$profileLeaf       = Split-Path $PROFILE -Leaf
$documentsDir      = Split-Path (Split-Path $PROFILE -Parent) -Parent
$candidateProfiles = @(
    $PROFILE
    Join-Path $documentsDir "WindowsPowerShell\$profileLeaf"
    Join-Path $documentsDir "PowerShell\$profileLeaf"
) | Select-Object -Unique

$anyProfileFailed = $false
foreach ($targetProfile in $candidateProfiles) {
    Write-Step "Checking PowerShell profile ($targetProfile) ..."

    # Use raw .NET IO instead of Test-Path/New-Item: on some OneDrive-synced
    # profile paths those cmdlets can report a directory/file as present when
    # it isn't yet materialized, causing New-Item to fail with a misleading
    # "could not find file" error. System.IO talks to the filesystem directly
    # and creates missing directories/files as needed.
    try {
        $targetDir = Split-Path $targetProfile -Parent
        if ($targetDir) {
            [System.IO.Directory]::CreateDirectory($targetDir) | Out-Null
        }

        $profileContent = ''
        if ([System.IO.File]::Exists($targetProfile)) {
            # Clear a stray ReadOnly attribute before attempting to write -
            # otherwise WriteAllText fails with a generic "Access to the
            # path is denied" error even though the file is readable.
            $attrs = [System.IO.File]::GetAttributes($targetProfile)
            if ($attrs -band [System.IO.FileAttributes]::ReadOnly) {
                [System.IO.File]::SetAttributes($targetProfile, ($attrs -band (-bnot [System.IO.FileAttributes]::ReadOnly)))
            }
            $profileContent = [System.IO.File]::ReadAllText($targetProfile)
        } elseif ($targetProfile -ne $PROFILE) {
            # Don't create a fresh profile for an edition that isn't
            # installed/used yet - only clean up/hook editions that already
            # have a profile file (or are the current host's $PROFILE).
            continue
        }

        # Drop any stale hook line from before the pw.ps1 -> pw-profile.ps1
        # rename so old profiles don't error trying to load a file that no
        # longer exists.
        $lines = $profileContent -split "`r?`n"
        $cleaned = $lines | Where-Object { $_ -notmatch $StaleHookPattern }
        if ($cleaned.Count -ne $lines.Count) {
            $profileContent = ($cleaned -join "`n")
            Write-Ok "Removed stale pw hook from $targetProfile"
        }

        if ($profileContent -like "*$HookLine*") {
            Write-Skip "pw hook already present in $targetProfile"
            if ($cleaned.Count -ne $lines.Count) {
                [System.IO.File]::WriteAllText($targetProfile, $profileContent)
            }
        } else {
            # Prepend the execution-policy bypass so an unsigned
            # pw-profile.ps1 loads under a default/Restricted policy, with
            # no manual step and no persisted config file (Process scope
            # only).
            $profileContent += "`n$HookComment`n$PolicyLine`n$HookLine`n"
            [System.IO.File]::WriteAllText($targetProfile, $profileContent)
            Write-Ok "Added pw hook to $targetProfile"
        }
    } catch {
        $anyProfileFailed = $true
        Write-Host "[warn] Could not update PowerShell profile automatically: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "       This can happen on some OneDrive-synced Documents folders (corporate policy or sync quirk)." -ForegroundColor Yellow
        Write-Host "       pw.exe and pw-profile.ps1 are installed; just add the pw hook manually:" -ForegroundColor Yellow
        Write-Host "         1. In File Explorer, go to: $targetDir" -ForegroundColor Yellow
        Write-Host "            (create the folder there if it doesn't exist)" -ForegroundColor Yellow
        Write-Host "         2. Create/open the file: $profileLeaf" -ForegroundColor Yellow
        Write-Host "         3. Add these two lines and save:" -ForegroundColor Yellow
        Write-Host "              $PolicyLine" -ForegroundColor Yellow
        Write-Host "              $HookLine" -ForegroundColor Yellow
    }
}
$profileWriteFailed = $anyProfileFailed

# ---------------------------------------------------------------------------
# Step 6 - Ensure ~/go/bin is on the user's PATH
# ---------------------------------------------------------------------------
# Adds BinDir to the persisted User PATH so pw.exe resolves by name even if
# the profile hook above didn't take (e.g. profile not sourced yet, or the
# `pw` function isn't loaded in the current session).
try {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $userPath) { $userPath = '' }
    $pathEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
    $alreadyOnPath = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $BinDir.TrimEnd('\') }
    if ($alreadyOnPath) {
        Write-Skip "$BinDir already on PATH"
    } else {
        $newPath = if ($userPath -and -not $userPath.EndsWith(';')) { "$userPath;$BinDir" } else { "$userPath$BinDir" }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Ok "Added $BinDir to your User PATH (restart your terminal to pick it up)"
    }
} catch {
    Write-Host "[warn] Could not update PATH automatically: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "       Add $BinDir to your PATH manually if you want to run pw.exe by name from any directory." -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host 'pw installed successfully.' -ForegroundColor Green
if ($profileWriteFailed) {
    Write-Host 'NOTE: the pw hook could not be added to your profile automatically - see the' -ForegroundColor Yellow
    Write-Host '      manual steps above. Until then, "pw" will not cd/launch correctly.' -ForegroundColor Yellow
} else {
    Write-Host "Restart your PowerShell session (or run: . `$PROFILE) to activate." -ForegroundColor Yellow
}
