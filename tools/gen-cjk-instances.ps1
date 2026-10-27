# 从系统安装的 Noto Sans/Serif SC 可变字体（VF）生成静态字重实例到 libs/fonts/。
#
# 背景：Google 的 NotoSansSC-VF.ttf / NotoSerifSC-VF.ttf 的 fvar 默认字重是
# Thin(100)，而 SDL2_ttf（FreeType）不支持 VF 实例化——直接打开渲染出的是
# Thin 正文，与 Chrome（DirectWrite 按命名实例取 400/700）差三档。引擎的
# backend/sdl2 会优先使用本脚本产出的静态 Regular/Bold 面（见 cjkNotoInstance）。
#
# 用法: powershell -ExecutionPolicy Bypass -File tools\gen-cjk-instances.ps1
# 依赖: python + fonttools（pip install fonttools）
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

$outDir = Join-Path $PSScriptRoot '..\libs\fonts'
New-Item -ItemType Directory -Force $outDir | Out-Null

# 源 VF → 目标静态面（族名, 字重值, 输出文件名）
$jobs = @(
    @{ Vf = 'NotoSansSC-VF.ttf';   Out = 'NotoSansSC-Regular.ttf';   Wght = 400 },
    @{ Vf = 'NotoSansSC-VF.ttf';   Out = 'NotoSansSC-Bold.ttf';      Wght = 700 },
    @{ Vf = 'NotoSerifSC-VF.ttf';  Out = 'NotoSerifSC-Regular.ttf';  Wght = 400 },
    @{ Vf = 'NotoSerifSC-VF.ttf';  Out = 'NotoSerifSC-Bold.ttf';     Wght = 700 }
)

python -c "import fontTools" 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Error '缺少 fonttools：先执行 pip install fonttools'
}

foreach ($j in $jobs) {
    $src = Join-Path $env:WINDIR "Fonts\$($j.Vf)"
    if (-not (Test-Path $src)) {
        Write-Host "skip (系统未安装 $($j.Vf)): $($j.Out)"
        continue
    }
    $dst = Join-Path $outDir $j.Out
    python -m fontTools.varLib.instancer $src "wght=$($j.Wght)" -o $dst
    if ($LASTEXITCODE -ne 0) { Write-Error "实例化失败: $($j.Out)" }
    Write-Host ("done: {0} ({1:N1} MB)" -f $j.Out, ((Get-Item $dst).Length / 1MB))
}
Write-Host "输出目录: $outDir（.gitignore 已排除，勿提交）"
