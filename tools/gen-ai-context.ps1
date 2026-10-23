# 重新生成 docs/ai/api-reference.md（从 go doc -all 导出全部非 examples 包的公开 API）。
# 用法: powershell -ExecutionPolicy Bypass -File tools\gen-ai-context.ps1
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

$pkgs = go list ./...
$out = New-Object System.Text.StringBuilder
$null = $out.AppendLine('# API 参考（自动生成）')
$null = $out.AppendLine('')
$null = $out.AppendLine('> 本文件由 `tools/gen-ai-context.ps1` 从 `go doc -all` 自动生成，请勿手工编辑；接口变化后重新运行脚本即可。')
$null = $out.AppendLine(('> 生成时间: ' + (Get-Date -Format 'yyyy-MM-dd HH:mm')))
foreach ($p in $pkgs) {
    $rel = $p -replace '^github.com/go-comm/ghromex/', '' -replace '^github.com/go-comm/ghromex$', '.'
    if ($rel.StartsWith('examples')) { continue }
    $null = $out.AppendLine("`n---`n")
    $null = $out.AppendLine("# package $rel")
    $null = $out.AppendLine('````')
    $d = go doc -all $p 2>$null
    $null = $out.AppendLine(($d -join "`n"))
    $null = $out.AppendLine('````')
}
$dst = Join-Path $PSScriptRoot '..\docs\ai\api-reference.md'
New-Item -ItemType Directory -Force (Split-Path $dst) | Out-Null
[System.IO.File]::WriteAllText($dst, $out.ToString(), [System.Text.UTF8Encoding]::new($false))
Write-Host "written: $dst ($((Get-Item $dst).Length) bytes)"
