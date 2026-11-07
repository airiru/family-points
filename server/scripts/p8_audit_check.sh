#!/usr/bin/env bash
# P8 审计日志 + 站内通知 验收
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
grant() { ( cd "$(dirname "$0")/.." && "$TOOL" -member "$1" -points "$2" ) > /dev/null 2>&1; }

echo "== 1. 造数据 =="
L=$(reg "l8$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
FID=$(echo "$L" | jget "['data']['member']['family_id']")
M=$(reg "m8$RUN" "小孩" false); M_AT=$(echo "$M" | jget "['data']['token']['access_token']"); M_ID=$(echo "$M" | jget "['data']['member']['id']")
A_AT=$(admin_login "$B")
curl -s -o /dev/null -X POST $B/api/admin/family/$FID/invite -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"username\":\"m8$RUN\"}"
grant "l8$RUN" 100
echo "  组=$FID"

echo "== 2. 触发若干写操作（应各留一条审计）=="
curl -s -o /dev/null -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"冰淇淋","cost":10}'
ITEM=$(curl -s $B/api/app/item/list -H "Authorization: Bearer $L_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/item/redeem -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM}"
curl -s -o /dev/null -X POST $B/api/app/transfer/send -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$M_ID,\"amount\":7}"
curl -s -o /dev/null -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"倒垃圾","points":10}'
curl -s -o /dev/null -X POST $B/api/app/rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"按时睡觉","points":3,"streaks":[{"every":5,"bonus":8,"mode":"repeat"}]}'

echo "== 3. 审计查询 =="
AU=$(curl -s "$B/api/admin/audit/list?family_id=$FID&limit=100" -H "Authorization: Bearer $A_AT")
TOTAL=$(echo "$AU" | jget "['data']['total']")
echo "  本组审计 $TOTAL 条"
chk "审计 ≥5 条" "True" "$([ "$TOTAL" -ge 5 ] && echo True || echo False)"
for a in item.redeem transfer.send task.create rule.save; do
  chk "有 $a 记录" "True" "$(echo "$AU" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['action']=='$a' for r in d['data']['items']))")"
done
chk "按 action 过滤只返回该动作" "1" "$(curl -s "$B/api/admin/audit/list?family_id=$FID&action=item.redeem" -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(len({r['action'] for r in d['data']['items']}))")"

echo "== 4. 脱敏：审计里不得出现密码哈希 =="
ALL=$(curl -s "$B/api/admin/audit/list?limit=200" -H "Authorization: Bearer $A_AT")
chk "不含 pass_hash 字样" "False" "$(echo "$ALL" | python -c "import sys,json;d=json.load(sys.stdin);print('pass_hash' in json.dumps(d))")"
chk "不含 bcrypt 哈希前缀" "False" "$(echo "$ALL" | python -c "import sys,json;d=json.load(sys.stdin);print('\$2a\$' in json.dumps(d) or '\$2b\$' in json.dumps(d))")"

echo "== 5. 审计是后台专属 + 只读 =="
chk "成员查审计 → 401" "401" "$(code "$B/api/admin/audit/list" -H "Authorization: Bearer $M_AT")"
chk "无审计删除接口 → 404" "404" "$(code -X DELETE "$B/api/admin/audit/1" -H "Authorization: Bearer $A_AT")"
chk "无审计修改接口 → 404" "404" "$(code -X PUT "$B/api/admin/audit/1" -H "Authorization: Bearer $A_AT")"

echo "== 6. 站内通知（单一写入路径，无重复）=="
N1=$(curl -s "$B/api/app/notification/list" -H "Authorization: Bearer $M_AT")
chk "受赠方收到 transfer_in" "True" "$(echo "$N1" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['type']=='transfer_in' for r in d['data']['items']))")"
chk "赠送通知恰好 1 条" "1" "$(echo "$N1" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='transfer_in'))")"
chk "旧 transfer 类型不再产生" "0" "$(echo "$N1" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='transfer'))")"
chk "赠送方没有收到（只发给受赠方）" "False" "$(curl -s "$B/api/app/notification/list" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['type']=='transfer_in' for r in d['data']['items']))")"
NID=$(echo "$N1" | python -c "import sys,json;d=json.load(sys.stdin);print([r['id'] for r in d['data']['items'] if r['type']=='transfer_in'][0])")
chk "标记单条已读 → 200" "200" "$(code -X POST $B/api/app/notification/read -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"id\":$NID}")"
chk "已读后 unread=1 不含它" "False" "$(curl -s "$B/api/app/notification/list?unread=1" -H "Authorization: Bearer $M_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['id']==$NID for r in d['data']['items']))")"
chk "标记别人的通知 → 404" "404" "$(code -X POST $B/api/app/notification/read -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"id\":$NID}")"
chk "全部已读 → 200" "200" "$(code -X POST $B/api/app/notification/read -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"all":true}')"

echo "== 7. 任务审核通知（完成者收到结果）=="
T=$(curl -s -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"洗碗","points":5}')
TID=$(echo "$T" | jget "['data']['id']")
curl -s -o /dev/null -X POST $B/api/app/task/claim -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$TID}"
S=$(curl -s -X POST $B/api/app/task/submit -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$TID,\"note\":\"洗完了\"}")
SID=$(echo "$S" | jget "['data']['id']")
curl -s -o /dev/null -X POST $B/api/app/task/vote -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SID,\"vote\":\"approve\"}"
N2=$(curl -s "$B/api/app/notification/list" -H "Authorization: Bearer $M_AT")
chk "审核结果通知到完成者（task_decided）" "True" "$(echo "$N2" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['type']=='task_decided' for r in d['data']['items']))")"
chk "审核结果通知恰好 1 条" "1" "$(echo "$N2" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='task_decided'))")"
chk "旧 task_review 类型不再产生" "0" "$(echo "$N2" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='task_review'))")"

echo "== 8. 登录 / 改密审计（auth.login / auth.password） =="
X=$(reg "x8$RUN" "审计号" true)
X_ID=$(echo "$X" | jget "['data']['member']['id']")
X_FID=$(echo "$X" | jget "['data']['member']['family_id']")
LOGIN_BODY="{\"username\":\"x8$RUN\",\"password\":\"Kid#2026pass\"}"
curl -s -o /dev/null -X POST $B/api/app/login -H 'Content-Type: application/json' -d "$LOGIN_BODY"
X_AT=$(curl -s -X POST $B/api/app/login -H 'Content-Type: application/json' -d "$LOGIN_BODY" | jget "['data']['token']['access_token']")
AQ="$B/api/admin/audit/list?family_id=$X_FID&operator_id=$X_ID&limit=50"
AX=$(curl -s "$AQ" -H "Authorization: Bearer $A_AT")
chk "成员登录落 auth.login（2 次成功登录）" "2" "$(echo "$AX" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['action']=='auth.login'))")"
chk "登录审计 operator_type=member" "True" "$(echo "$AX" | python -c "import sys,json;d=json.load(sys.stdin);print(all(r['operator_type']=='member' for r in d['data']['items']))")"
chk "登录审计 family_id 与本组一致" "True" "$(echo "$AX" | python -c "import sys,json;d=json.load(sys.stdin);print(all(r['family_id']==$X_FID for r in d['data']['items']))")"
chk "登录失败 → 401" "401" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"x8$RUN\",\"password\":\"wrong\"}")"
chk "登录失败不写审计" "2" "$(curl -s "$AQ" -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['action']=='auth.login'))")"
chk "改密 → 200" "200" "$(code -X POST $B/api/app/password -H "Authorization: Bearer $X_AT" -H 'Content-Type: application/json' -d '{"old_password":"Kid#2026pass","new_password":"New#2026pass"}')"
AX2=$(curl -s "$AQ" -H "Authorization: Bearer $A_AT")
chk "改密落 auth.password" "1" "$(echo "$AX2" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['action']=='auth.password'))")"
chk "认证审计 before/after 无密码或 token 字段" "False" "$(echo "$AX2" | python -c "import sys,json;d=json.load(sys.stdin);print(any(any(k in (r.get('before') or '')+(r.get('after') or '') for k in ['password','pass_hash','token']) for r in d['data']['items']))")"
chk "管理员登录落 auth.login（system）" "True" "$(curl -s "$B/api/admin/audit/list?action=auth.login&limit=200" -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['operator_type']=='system' for r in d['data']['items']))")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
