function pw {
    $out = & pw.exe @args
    if ($LASTEXITCODE -eq 0 -and $out) {
        $dir = $out[0]
        $act = if ($out.Length -gt 1) { $out[1] } else { "" }
        $ed  = if ($out.Length -gt 2) { $out[2] } else { "" }
        if ($dir) {
            Set-Location -LiteralPath $dir
        }
        if ($act -eq "opencode") {
            opencode
        } elseif ($act -eq "editor" -and $ed) {
            & $ed .
        }
    }
}
