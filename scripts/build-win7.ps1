# build-win7.ps1 -- produce a Windows 7 compatible demo build.
#
# Background: Go 1.21+ officially dropped Windows 7/8/8.1 support; binaries
# built with them crash at runtime on Win7 (rip=0 access violation). Go 1.20
# is the last release that runs on Win7. This script copies the source tree
# into .temp\win7build, relaxes go.mod there (go 1.20 + golang.org/x/sys
# v0.8.0, which still supports go 1.17+) and builds with a Go 1.20.x toolchain.
# The main working copy is never modified.
#
# Usage (from the project root, zero args, toolchain auto-detected):
#   powershell -ExecutionPolicy Bypass -File scripts\build-win7.ps1
#
# Requires a Go 1.20.x toolchain (do NOT need it on PATH). Resolution order:
#   1. -Go120 <path>\bin\go.exe              explicit override
#   2. `g` (Golang Version Manager) versions: %USERPROFILE%\.g\versions\1.20*
#   3. D:\local\lib\go-1.20\bin\go.exe       manual install fallback
# Install via g if missing:  g install 1.20.14
#   (download mirror: https://golang.google.cn/dl/go1.20.14.windows-amd64.zip)

param(
    [string]$Go120 = ''
)
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot   # project root (script lives in scripts\)

if (-not $Go120) {
    # Auto-detect: newest patch first among g-managed 1.20.x versions.
    # NOTE @(): a single-item pipeline collapses to a scalar string, and
    # "+=" on a scalar CONCATENATES instead of appending to an array.
    $candidates = @()
    $gVersions = Join-Path $env:USERPROFILE '.g\versions'
    if (Test-Path -LiteralPath $gVersions) {
        $candidates = @(Get-ChildItem -LiteralPath $gVersions -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -like '1.20*' } | Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName 'bin\go.exe' })
    }
    $candidates += 'D:\local\lib\go-1.20\bin\go.exe'
    foreach ($c in $candidates) {
        if (Test-Path -LiteralPath $c) { $Go120 = $c; break }
    }
}
if (-not $Go120 -or -not (Test-Path -LiteralPath $Go120)) {
    Write-Error "No Go 1.20 toolchain found.`nInstall one with the version manager:  g install 1.20.14`nor pass -Go120 <path>\bin\go.exe"
}

# The machine-wide GOROOT usually points at the primary (newer) toolchain; a
# Go 1.20 go.exe would inherit it and fail with "version go1.25.x does not
# match go tool version". Pin GOROOT to this toolchain for the whole session.
$Go120 = (Resolve-Path -LiteralPath $Go120).Path
$env:GOROOT = Split-Path -Parent (Split-Path -Parent $Go120)

$build = Join-Path $root '.temp\win7build'
if (Test-Path -LiteralPath $build) { Remove-Item -LiteralPath $build -Recurse -Force }
New-Item -ItemType Directory -Path $build | Out-Null

foreach ($d in 'engine', 'components', 'demo', 'backend') {
    Copy-Item -LiteralPath (Join-Path $root $d) -Destination $build -Recurse
}
Copy-Item -LiteralPath (Join-Path $root 'go.mod'), (Join-Path $root 'go.sum') -Destination $build

Push-Location $build
try {
    # Go 1.20's go.mod parser rejects three-segment directives ("go 1.25.0")
    # and the "-go=1.20" equals form, so rewrite the go line as text instead
    # (go.mod is pure ASCII; write BOM-free UTF-8 to stay go-parser friendly).
    $gomod = Join-Path (Get-Location).Path 'go.mod'
    $text = [IO.File]::ReadAllText($gomod)
    $text = [regex]::Replace($text, '(?m)^go .+$', 'go 1.20')
    [IO.File]::WriteAllText($gomod, $text, (New-Object System.Text.UTF8Encoding($false)))

    # x/sys v0.33.0 requires go >= 1.23; v0.8.0 is the last line with a low
    # go directive and still exposes every API this project uses.
    & $Go120 get golang.org/x/sys@v0.8.0
    if ($LASTEXITCODE -ne 0) { throw "go get x/sys@v0.8.0 failed ($LASTEXITCODE). Check GOPROXY." }
    & $Go120 mod tidy
    if ($LASTEXITCODE -ne 0) { throw "go mod tidy failed ($LASTEXITCODE)" }

    & $Go120 vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed ($LASTEXITCODE)" }
    $exe = Join-Path $build 'demo-win7.exe'
    & $Go120 build -trimpath -o $exe ./demo
    if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" }
}
finally {
    Pop-Location
}

# Assemble a ready-to-copy dist: exe + its libs folder (SDL2 2.28.4 / SDL2_ttf
# 2.20.1 / zlib are Win7-compatible). findLibsDir() picks up <exe-dir>\libs.
$dist = Join-Path $root '.temp\win7dist'
if (Test-Path -LiteralPath $dist) { Remove-Item -LiteralPath $dist -Recurse -Force }
New-Item -ItemType Directory -Path (Join-Path $dist 'libs') | Out-Null
Copy-Item -LiteralPath $exe -Destination $dist
Copy-Item -LiteralPath (Join-Path $root 'libs\SDL2.dll'), (Join-Path $root 'libs\SDL2_ttf.dll'), (Join-Path $root 'libs\zlib1.dll') -Destination (Join-Path $dist 'libs')

# Drop the transient source copy; keep the dist folder for deployment.
Remove-Item -LiteralPath $build -Recurse -Force

Write-Output "OK: $dist"
Write-Output "Copy the whole win7dist folder to the Win7 machine, run demo-win7.exe."
