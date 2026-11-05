#!/usr/bin/env bash
# 管理员登录的公共实现。
#
# 为什么需要：回归脚本会**改管理员密码**（p2 改密流程、p8b 也会），
# 任何脚本把密码写死成一个值，都会在某个时点集体假红 —— 看起来像代码回归，
# 实际只是脚本脆弱。这里统一按「环境变量优先 + 历史默认值兜底」尝试。
#
# 用法：. "$(dirname "$0")/lib_admin.sh"  然后  A_AT=$(admin_login "$B")

admin_login() {
  local B="$1" pwd at resp
  local CAND=()
  [ -n "${ADMIN_PASSWORD:-}" ] && CAND+=("$ADMIN_PASSWORD")
  CAND+=(admins1234 admins123 admins)

  for pwd in "${CAND[@]}"; do
    resp=$(curl -s -X POST "$B/api/admin/login" -H 'Content-Type: application/json' \
      -d "{\"username\":\"admins\",\"password\":\"$pwd\"}")
    at=$(echo "$resp" | python -c "import sys,json;print(json.load(sys.stdin)['data']['token']['access_token'])" 2>/dev/null)
    if [ -n "${at:-}" ]; then
      echo "$at"
      return 0
    fi
  done
  echo "❌ 管理员登录失败：已尝试 ADMIN_PASSWORD / admins1234 / admins123 / admins" >&2
  return 1
}
