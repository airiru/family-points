#!/usr/bin/env bash
# P10 登出验收（对应 5.4.4 退出 + refresh 吊销）
#
# 核心断言：**登出之后，登出前拿到的 refresh token 不能再换出 access**。
# 这条是之前的真实缺口——前端「丢 token」不等于服务端失效，refresh 还能用 30 天。
set -u
. "$(dirname "$0")/lib_admin.sh"
B=http://127.0.0.1:8080
RUN=$(date +%s)
KID="p10kid$RUN"
pass=0; fail=0

chk() { # chk <名称> <期望> <实际>
  if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1));
  else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi
}
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

echo "== 1. 前台：注册 → 登出 → 旧 refresh 重放必须被拒 =="
REG=$(curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d '{"username":"'$KID'","password":"Kid#2026pass","name":"小明","create_family":true,"family_name":"登出测试组"}')
M_AT=$(echo "$REG" | jget "['data']['token']['access_token']")
M_RT=$(echo "$REG" | jget "['data']['token']['refresh_token']")
[ -n "${M_AT:-}" ] && echo "  ✅ 注册成功" && pass=$((pass+1)) || { echo "  ❌ 注册失败"; fail=$((fail+1)); }

chk "登出前 refresh 可用" "200" "$(code -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$M_RT\"}")"
# 上一步轮换会吊销旧 RT，重新登录取一份干净的
LOGIN=$(curl -s -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"$KID\",\"password\":\"Kid#2026pass\"}")
M_AT=$(echo "$LOGIN" | jget "['data']['token']['access_token']")
M_RT=$(echo "$LOGIN" | jget "['data']['token']['refresh_token']")

chk "登出接口 200" "200" "$(code -X POST $B/api/app/logout -H "Authorization: Bearer $M_AT")"
chk "登出后旧 refresh 被拒" "401" "$(code -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$M_RT\"}")"
chk "登出后旧 access 仍可用（无状态，到期为止）" "200" "$(code $B/api/app/me -H "Authorization: Bearer $M_AT")"

echo "== 2. 前台：未登录调 logout 必须 401 =="
chk "无 token logout → 401" "401" "$(code -X POST $B/api/app/logout)"

echo "== 3. 后台：登出 → 旧 refresh 重放必须被拒 =="
# 需要 access + refresh 一对：先用 lib_admin 登录（它会自动处理首次强制改密），
# 再用它导出的 ADMIN_PWD_USED 登录一次拿 refresh。
A_AT=$(admin_login "$B")
A_RT=""
if [ -n "${A_AT:-}" ]; then
  ADM=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' \
    -d "{\"username\":\"admins\",\"password\":\"${ADMIN_PWD_USED:-admins}\"}")
  A_RT=$(echo "$ADM" | jget "['data']['token']['refresh_token']")
fi
[ -n "${A_AT:-}" ] && echo "  ✅ 后台登录成功" && pass=$((pass+1)) || { echo "  ❌ 后台登录失败"; fail=$((fail+1)); }

chk "后台登出接口 200" "200" "$(code -X POST $B/api/admin/logout -H "Authorization: Bearer $A_AT")"
chk "后台登出后旧 refresh 被拒" "401" "$(code -X POST $B/api/admin/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$A_RT\"}")"

echo "== 4. 隔离：前台 token 打后台 logout 必须 401 =="
L2=$(curl -s -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"$KID\",\"password\":\"Kid#2026pass\"}")
M2_AT=$(echo "$L2" | jget "['data']['token']['access_token']")
chk "member → admin/logout 401" "401" "$(code -X POST $B/api/admin/logout -H "Authorization: Bearer $M2_AT")"

echo "== 5. 登出后重新登录仍可正常使用（不是把账号锁死）=="
L3=$(curl -s -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"$KID\",\"password\":\"Kid#2026pass\"}")
M3_AT=$(echo "$L3" | jget "['data']['token']['access_token']")
chk "重新登录后 app/me 200" "200" "$(code $B/api/app/me -H "Authorization: Bearer $M3_AT")"
M3_RT=$(echo "$L3" | jget "['data']['token']['refresh_token']")
chk "重新登录后 refresh 可用" "200" "$(code -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$M3_RT\"}")"

echo
echo "P10 登出：通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ] || exit 1
