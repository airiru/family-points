#!/usr/bin/env bash
# P8b 后台管理接口 + CSV 导出 验收
set -u
. "$(dirname "$0")/lib_admin.sh"
B=http://127.0.0.1:8080
RUN=$(date +%s)
TOOL=/tmp/devtool.exe
[ -x "$TOOL" ] || TOOL=/tmp/devtool
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
reg() { curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"; }

A_AT=$(admin_login "$B")
AH="Authorization: Bearer $A_AT"

echo "== 1. 造数据 =="
L=$(reg "l9$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']"); L_ID=$(echo "$L" | jget "['data']['member']['id']")
FID=$(curl -s $B/api/app/family -H "Authorization: Bearer $L_AT" | jget "['data']['id']")
# 一个未入组的用户，供后台邀请
OUT=$(reg "o9$RUN" "待邀请" false); OUT_AT=$(echo "$OUT" | jget "['data']['token']['access_token']"); OUT_ID=$(echo "$OUT" | jget "['data']['member']['id']")
echo "  组=$FID 组长=$L_ID 待邀请=$OUT_ID"

echo "== 2. 全站视图 =="
chk "家庭组列表 200" "200" "$(code $B/api/admin/families -H "$AH")"
chk "列表含本组" "True" "$(curl -s $B/api/admin/families -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(any(f['id']==$FID for f in d['data']['families']))")"
chk "列表带成员数" "True" "$(curl -s $B/api/admin/families -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(all('member_count' in f for f in d['data']['families']))")"
chk "全站统计 200" "200" "$(code $B/api/admin/state -H "$AH")"
chk "统计有 family_count" "True" "$(curl -s $B/api/admin/state -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(d['data']['family_count']>0)")"

echo "== 3. 后台成员管理 =="
chk "邀请未入组用户 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"o9$RUN\"}")"
chk "被邀请者已入组" "$FID" "$(curl -s $B/api/app/me -H "Authorization: Bearer $OUT_AT" | jget "['data']['family_id']")"
chk "重复邀请 → 409" "409" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"o9$RUN\"}")"
chk "邀请不存在的用户 → 400" "400" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d '{"username":"nobody_here_x"}')"
chk "踢出组长 → 409（须先换组长）" "409" "$(code -X POST $B/api/admin/family/$FID/member/kick -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID}")"
chk "后台设新组长 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/member/leader -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$OUT_ID}")"
chk "新组长角色生效" "leader" "$(curl -s $B/api/app/me -H "Authorization: Bearer $OUT_AT" | jget "['data']['role']")"
chk "原组长降为 member" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $L_AT" | jget "['data']['role']")"
chk "隐藏成员 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/member/hidden -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID,\"hidden\":true}")"
chk "隐藏后成员列表不含他" "False" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $OUT_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(m['id']==$L_ID for m in d['data']['members']))")"
chk "后台改组名 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/rename -H "$AH" -H 'Content-Type: application/json' -d '{"name":"后台改的名"}')"
chk "组名已改" "后台改的名" "$(curl -s $B/api/app/family -H "Authorization: Bearer $OUT_AT" | jget "['data']['name']")"

echo "== 4. 后台配置（与组长同权）=="
chk "后台存规则 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/rule/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"按时睡觉","points":4,"streaks":[{"every":2,"bonus":3,"mode":"repeat"}]}')"
chk "规则对成员生效" "4" "$(curl -s $B/api/app/rule -H "Authorization: Bearer $OUT_AT" | jget "['data']['rules'][0]['points']")"
chk "后台非法档位 → 400" "400" "$(code -X POST $B/api/admin/family/$FID/rule/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"x","points":1,"streaks":[{"every":0,"bonus":1}]}')"
chk "后台存商品 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/item/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"看电影","cost":20}')"
chk "商品对成员可见" "看电影" "$(curl -s $B/api/app/item/list -H "Authorization: Bearer $OUT_AT" | jget "['data']['items'][0]['name']")"

echo "== 5. CSV 导出 =="
HDR=$(curl -s -D - -o /tmp/exp.csv "$B/api/admin/export/records?family_id=$FID" -H "$AH" | tr -d '\r')
chk "导出 200 + text/csv" "True" "$(echo "$HDR" | grep -qi 'text/csv' && echo True || echo False)"
chk "带附件头" "True" "$(echo "$HDR" | grep -qi 'attachment' && echo True || echo False)"
chk "CSV 有表头" "时间,成员,标题,分值,来源" "$(head -1 /tmp/exp.csv | sed 's/^\xEF\xBB\xBF//')"
chk "缺 family_id → 400" "400" "$(code "$B/api/admin/export/records" -H "$AH")"

echo "== 6. 管理员账号 =="
chk "账号列表 200" "200" "$(code $B/api/admin/accounts -H "$AH")"
chk "列表含 admins" "True" "$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(any(a['username']=='admins' for a in d['data']['admins']))")"
chk "列表不含 pass_hash" "False" "$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;print('pass_hash' in json.dumps(json.load(sys.stdin)))")"
chk "新增管理员 → 200" "200" "$(code -X POST $B/api/admin/accounts -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}")"
NEW_ID=$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print([a['id'] for a in d['data']['admins'] if a['username']=='ops$RUN'][0])")
chk "新管理员首登被要求改密" "True" "$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}" | jget "['data']['token']['must_change_password']")"
ME_ID=$(curl -s $B/api/admin/me -H "$AH" | jget "['data']['id']")
chk "停用自己 → 400" "400" "$(code -X POST $B/api/admin/accounts/disable -H "$AH" -H 'Content-Type: application/json' -d "{\"admin_id\":$ME_ID,\"disabled\":true}")"
chk "停用新账号 → 200" "200" "$(code -X POST $B/api/admin/accounts/disable -H "$AH" -H 'Content-Type: application/json' -d "{\"admin_id\":$NEW_ID,\"disabled\":true}")"
chk "停用后登录 → 403" "403" "$(code -X POST $B/api/admin/login -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}")"

echo "== 7. 越权 =="
chk "成员 token 打后台 → 401" "401" "$(code $B/api/admin/families -H "Authorization: Bearer $OUT_AT")"
chk "成员 token 导出 → 401" "401" "$(code "$B/api/admin/export/records?family_id=$FID" -H "Authorization: Bearer $OUT_AT")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
