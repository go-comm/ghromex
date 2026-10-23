#!/usr/bin/env bash
# 重新生成 docs/ai/api-reference.md（从 go doc -all 导出全部非 examples 包的公开 API）。
# 用法: bash tools/gen-ai-context.sh
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

module_path="$(go list -m)"
[ -n "$module_path" ] || { echo "go list -m failed" >&2; exit 1; }

pkgs="$(go list ./...)" || { echo "go list ./... failed" >&2; exit 1; }

dst="docs/ai/api-reference.md"
mkdir -p "$(dirname "$dst")"

fence='````'

{
  echo '# API 参考（自动生成）'
  echo ''
  echo '> 本文件由 `tools/gen-ai-context.sh` 从 `go doc -all` 自动生成，请勿手工编辑；接口变化后重新运行脚本即可。'
  echo "> 生成时间: $(date '+%Y-%m-%d %H:%M')"
} > "$dst"

while IFS= read -r p; do
  [ -n "$p" ] || continue
  if [ "$p" = "$module_path" ]; then
    rel='.'
  else
    rel="${p#"$module_path"/}"
  fi
  case "$rel" in
    examples|examples/*) continue ;;
  esac
  {
    echo ''
    echo '---'
    echo ''
    echo "# package $rel"
    echo "$fence"
    if ! go doc -all "$p" 2>/dev/null; then
      echo "go doc failed for $p" >&2
      echo "(go doc failed for $p)"
    fi
    echo "$fence"
  } >> "$dst"
done <<< "$pkgs"

bytes="$(wc -c < "$dst" | tr -d ' ')"
echo "written: $dst ($bytes bytes)"
