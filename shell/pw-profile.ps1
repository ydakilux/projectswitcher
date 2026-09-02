function pw {
    # Force array coercion: PowerShell can collapse external-command output
    # into a plain string instead of an array under some conditions, which
    # would silently break the $out[0]/$out[1]/$out[2] indexing below.
    [string[]]$out = & pw.exe @args
    if ($LASTEXITCODE -eq 0 -and $out) {
        $dir = $out[0].Trim()
        $act = if ($out.Length -gt 1) { $out[1].Trim() } else { "" }
        $ed  = if ($out.Length -gt 2) { $out[2].Trim() } else { "" }
        if ($dir) {
            try {
                Set-Location -LiteralPath $dir -ErrorAction Stop
            } catch {
                Write-Host "pw: could not cd to '$dir': $($_.Exception.Message)" -ForegroundColor Red
                return
            }
        }
        if ($act -eq "opencode") {
            opencode
        } elseif ($act -eq "editor" -and $ed) {
            & $ed .
        }
    }
}
