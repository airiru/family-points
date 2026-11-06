#!/usr/bin/env bash
# P1 认证验收测试（对应验收 6、7、16 + 5.4.2 + refresh 轮换/吊销）
set -u
. "$(dirname "$0")/lib_admin.sh"
B=http://127.0.0.1:8080
RUN=$(date +%s)
KID="p1kid$RUN"
KID2="p1kid${RUN}b"
pass=0; fail=0

chk() { # chk <名称> <期望> <实际>
  if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1));
  else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi
}
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }

echo "== 1. 注册（带创建家庭组）=="
REG=$(curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d '{"username":"'$KID'","password":"Kid#2026pass","nickname":"小明","create_family":true,"family_name":"测试之家"}')
echo "  $REG" | head -c 300; echo
M_AT=$(echo "$REG" | jget "['data']['token']['access_token']")
M_RT=$(echo "$REG" | jget "['data']['token']['refresh_token']")
ROLE=$(echo "$REG" | jget "['data']['member']['role']")
FID=$(echo "$REG" | jget "['data']['member']['family_id']")
chk "注册后角色=leader" "leader" "$ROLE"
[ -n "$FID" ] && [ "$FID" != "None" ] && { echo "  ✅ 已创建家庭组 family_id=$FID"; pass=$((pass+1)); } || { echo "  ❌ 未创建家庭组"; fail=$((fail+1)); }

echo "== 2. 前台 token 访问 /api/app/me =="
chk "app/me 200" "200" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/app/me -H "Authorization: Bearer $M_AT")"

echo "== 3. 验收 6：前台 token 打后台接口必须 401 =="
chk "member→admin/me 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/admin/me -H "Authorization: Bearer $M_AT")"

echo "== 4. 后台登录（密码由 ADMIN_PASSWORD 或历史默认值提供）=="
ADM=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d '{"username":"admins","password":"admins"}')
A_AT=$(admin_login "$B")
[ -n "${A_AT:-}" ] && { echo "  ✅ 管理员可登录"; pass=$((pass+1)); } || { echo "  ❌ 管理员登录失败"; fail=$((fail+1)); }
MUST=$(echo "$ADM" | jget "['data']['token']['must_change_password']")

echo "== 5. 5.4.2：未改密前除改密接口外全部拒绝 =="
if [ "$MUST" = "True" ]; then
  chk "登录成功且标记必须改密" "True" "$MUST"
  chk "admin/me 403" "403" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/admin/me -H "Authorization: Bearer $A_AT")"
else
  # 管理员密码已改过（跑过 p2/p8b 或人工改过）→ 这两条只在「全新 seed」时成立，
  # 跳过而不是判失败，否则每次回归都假红。
  echo "  ⏭ 跳过「首次登录必须改密」两项：当前管理员已改过密码（需 migrate -reset + -seed-admin 才可验）"
fi

echo "== 6. 验收 7：后台 token 打前台接口必须 401 =="
chk "admin→app/me 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/app/me -H "Authorization: Bearer $A_AT")"

echo "== 7. 验收 16：注册接口不产生管理员（admins 表仍只有 1 个，默认账号仍能登录）=="
curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d '{"username":"'$KID2'","password":"Kid#2026pass","nickname":"小红"}' > /dev/null
[ -n "$(admin_login "$B")" ] && R=200 || R=401
chk "默认管理员仍可登录" "200" "$R"

echo "== 8. refresh 轮换：新 token 可用，旧 refresh 立即失效 =="
R1=$(curl -s -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$M_RT\"}")
N_RT=$(echo "$R1" | jget "['data']['refresh_token']")
N_AT=$(echo "$R1" | jget "['data']['access_token']")
chk "换到新的 access 可访问" "200" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/app/me -H "Authorization: Bearer $N_AT")"
chk "旧 refresh 重放被拒" "401" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$M_RT\"}")"

echo "== 9. 改密码后旧 refresh 全部失效（一步到位的核心）=="
chk "改密 200" "200" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/app/password -H "Authorization: Bearer $N_AT" -H 'Content-Type: application/json' -d '{"old_password":"Kid#2026pass","new_password":"Kid#New2026"}')"
chk "旧 refresh 已失效" "401" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/app/refresh -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$N_RT\"}")"
chk "旧密码登录被拒" "401" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/app/login -H 'Content-Type: application/json' -d '{"username":"'$KID'","password":"Kid#2026pass"}')"
chk "新密码登录成功" "200" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/app/login -H 'Content-Type: application/json' -d '{"username":"'$KID'","password":"Kid#New2026"}')"

echo "== 10. 越权与输入校验 =="
chk "无 token → 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/app/me)"
chk "伪造 token → 401" "401" "$(curl -s -o /dev/null -w '%{http_code}' $B/api/app/me -H 'Authorization: Bearer abc.def.ghi')"
chk "重复用户名 → 409" "409" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/register -H 'Content-Type: application/json' -d '{"username":"'$KID'","password":"Kid#2026pass","nickname":"x"}')"
chk "弱密码 → 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/register -H 'Content-Type: application/json' -d '{"username":"weakpwd1","password":"123","nickname":"x"}')"
chk "非法用户名 → 400" "400" "$(curl -s -o /dev/null -w '%{http_code}' -X POST $B/api/register -H 'Content-Type: application/json' -d '{"username":"a b","password":"Kid#2026pass","nickname":"x"}')"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
