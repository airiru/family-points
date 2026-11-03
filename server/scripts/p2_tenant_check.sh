#!/usr/bin/env bash
# P2 多租户隔离验收（对应验收 1–3；4/5 依赖配置接口，待 P3/P5）
# 幂等：每次用带时间戳的用户名，自建 A/B 两个组，不依赖历史数据。
set -u
B=http://127.0.0.1:8080
RUN=$(date +%s)
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

echo "== 0. 管理员登录（首次必须改密，5.4.2）=="
A_AT=""
for pwd in admins123 admins; do
  ADM=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d "{\"username\":\"admins\",\"password\":\"$pwd\"}")
  A_AT=$(echo "$ADM" | jget "['data']['token']['access_token']")
  if [ -n "${A_AT:-}" ] && [ "$A_AT" != "None" ]; then
    echo "  以密码 $pwd 登录成功"
    if [ "$pwd" = "admins" ]; then
      chk "首次登录标记必须改密" "True" "$(echo "$ADM" | jget "['data']['token']['must_change_password']")"
      chk "未改密时后台接口被拦" "403" "$(code $B/api/admin/me -H "Authorization: Bearer $A_AT")"
      chk "改密成功" "200" "$(code -X POST $B/api/admin/password -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{"old_password":"admins","new_password":"admins123"}')"
      ADM=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d '{"username":"admins","password":"admins123"}')
      A_AT=$(echo "$ADM" | jget "['data']['token']['access_token']")
    fi
    break
  fi
done
chk "must_change_password=false 时字段仍返回" "False" "$(echo "$ADM" | jget "['data']['token']['must_change_password']")"
chk "改密后后台接口可用" "200" "$(code $B/api/admin/me -H "Authorization: Bearer $A_AT")"

echo "== 1. 现场造 A / B 两个家庭组 =="
RA=$(curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"pa$RUN\",\"password\":\"Kid#2026pass\",\"name\":\"甲组家长\",\"create_family\":true,\"family_name\":\"甲组$RUN\"}")
FA_ID=$(echo "$RA" | jget "['data']['member']['family_id']")
FA_AT=$(echo "$RA" | jget "['data']['token']['access_token']")
RB=$(curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"pb$RUN\",\"password\":\"Kid#2026pass\",\"name\":\"乙组家长\",\"create_family\":true,\"family_name\":\"乙组$RUN\"}")
FB_ID=$(echo "$RB" | jget "['data']['member']['family_id']")
echo "  A 组 family_id=$FA_ID  B 组 family_id=$FB_ID"
chk "A/B 是两个不同的组" "True" "$([ "$FA_ID" != "$FB_ID" ] && echo True || echo False)"

echo "== 2. 验收 1/2：family_id 只认 token，篡改无效 =="
chk "A 看自己的组 → 200" "200" "$(code $B/api/app/family -H "Authorization: Bearer $FA_AT")"
chk "query 传 B 的 id 被忽略" "$FA_ID" "$(curl -s "$B/api/app/family?family_id=$FB_ID" -H "Authorization: Bearer $FA_AT" | jget "['data']['id']")"
chk "body 传 B 的 id 被忽略" "$FA_ID" "$(curl -s -X GET "$B/api/app/family" -H "Authorization: Bearer $FA_AT" -H 'Content-Type: application/json' -d "{\"family_id\":$FB_ID}" | jget "['data']['id']")"

echo "== 3. 验收 3：成员列表只含本组 =="
MEM_A=$(curl -s $B/api/app/family/members -H "Authorization: Bearer $FA_AT")
chk "A 组看不到 B 组的人" "False" "$(echo "$MEM_A" | python -c "import sys,json;d=json.load(sys.stdin);print(any(m['id']==$(echo "$RB" | jget "['data']['member']['id']") for m in d['data']['members']))")"

echo "== 4. 管理员跨组：必须显式路径参数（4.3 第 3 条）=="
chk "管理员看 B 组 → 200" "200" "$(code $B/api/admin/family/$FB_ID -H "Authorization: Bearer $A_AT")"
chk "拿到的是 B 组" "乙组$RUN" "$(curl -s $B/api/admin/family/$FB_ID -H "Authorization: Bearer $A_AT" | jget "['data']['name']")"
chk "不存在的组 → 404" "404" "$(code $B/api/admin/family/99999 -H "Authorization: Bearer $A_AT")"
chk "非法 id → 404（不返回 400）" "404" "$(code $B/api/admin/family/abc -H "Authorization: Bearer $A_AT")"

echo "== 5. 两套 token 互不通用 =="
chk "member token 打 admin 跨组接口 → 401" "401" "$(code $B/api/admin/family/$FB_ID -H "Authorization: Bearer $FA_AT")"
chk "admin token 打 app 接口 → 401" "401" "$(code $B/api/app/family -H "Authorization: Bearer $A_AT")"

echo "== 6. 未入组用户被挡在组内接口外 =="
NF=$(curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"pn$RUN\",\"password\":\"Kid#2026pass\",\"name\":\"无组人\"}")
NF_AT=$(echo "$NF" | jget "['data']['token']['access_token']")
chk "未入组访问 /api/app/family → 403" "403" "$(code $B/api/app/family -H "Authorization: Bearer $NF_AT")"
chk "未入组访问 /api/app/me → 200（不需要组）" "200" "$(code $B/api/app/me -H "Authorization: Bearer $NF_AT")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
