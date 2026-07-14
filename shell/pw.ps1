function pw {
    $dir = & pw.exe @args
    if ($LASTEXITCODE -eq 0 -and $dir) {
        Set-Location -LiteralPath $dir
    }
}
