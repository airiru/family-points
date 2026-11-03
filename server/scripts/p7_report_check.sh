#!/usr/bin/env bash
# P7 概览 / 记录查询 / 两个榜 / 趋势 验收
set -u
B=http://127.0.0.1:8080
RUN=$(date +%s)
TOOL=/tmp/devtool.exe
[ -x "$TOOL" ] || TOOL=/tmp/devtool
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
reg() { curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"name\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"; }
grant() { ( cd "$(dirname "$0")/.." && "$TOOL" -member "$1" -points "$2" ) > /dev/null 2>&1; }

TODAY=$(date +%Y-%m-%d)
D1=$(date -d "2 days ago" +%Y-%m-%d)
D2=$(date -d "1 day ago" +%Y-%m-%d)

echo "== 1. 造数据：L(50 分，无流水) + M1(靠屏幕上报得分) =="
L=$(reg "l7$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
CODE=$(curl -s $B/api/app/family/invite-code -H "Authorization: Bearer $L_AT" | jget "['data']['invite_code']")
M1=$(reg "m7$RUN" "小孩" false); M1_AT=$(echo "$M1" | jget "['data']['token']['access_token']"); M1_ID=$(echo "$M1" | jget "['data']['member']['id']")
curl -s -o /dev/null -X POST $B/api/app/family/join -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"invite_code\":\"$CODE\"}"
FID=$(curl -s $B/api/app/family -H "Authorization: Bearer $L_AT" | jget "['data']['id']")
grant "l7$RUN" 50
A_AT=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d '{"username":"admins","password":"admins123"}' | jget "['data']['token']['access_token']")
DEV=$(curl -s -X POST $B/api/admin/family/$FID/device -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M1_ID,\"name\":\"测试机\"}")
TOKEN=$(echo "$DEV" | jget "['data']['token']")
# 三天屏幕上报：每天 +2（日总量）+2（夜间）= +4
for d in "$D1" "$D2" "$TODAY"; do
  curl -s -o /dev/null -X POST $B/api/device/screen/report -H "X-Device-Token: $TOKEN" -H 'Content-Type: application/json' -d "{\"report_date\":\"$d\",\"total_min\":60,\"night_min\":0}"
done
echo "  今天=$TODAY 前两天=$D1 / $D2"

echo "== 2. 概览 /api/app/state =="
ST=$(curl -s $B/api/app/state -H "Authorization: Bearer $M1_AT")
chk "返回家庭组名" "组$RUN" "$(echo "$ST" | jget "['data']['family']['name']")"
chk "成员数 2" "2" "$(echo "$ST" | jget "['data']['members'].__len__()")"
chk "我的积分 17（4+4+9，第 3 天含 +5 档位）" "17" "$(echo "$ST" | jget "['data']['me']['score']")"
chk "概览不含全量记录" "False" "$(echo "$ST" | python -c "import sys,json;d=json.load(sys.stdin);print('records' in d['data'] or 'items' in d['data'])")"
chk "未读数 0" "0" "$(echo "$ST" | jget "['data']['unread_count']")"

echo "== 3. 记录查询：分页 / 筛选 / 搜索 =="
R=$(curl -s "$B/api/app/records?limit=2" -H "Authorization: Bearer $M1_AT")
chk "分页返回 2 条" "2" "$(echo "$R" | jget "['data']['items'].__len__()")"
chk "总数 6（3 天 × 日+夜）" "6" "$(echo "$R" | jget "['data']['total']")"
chk "按成员筛选（L 无流水 → 0）" "0" "$(curl -s "$B/api/app/records?member_id=1" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")"
chk "按来源筛选 night → 3" "3" "$(curl -s "$B/api/app/records?source=night" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")"
chk "关键词搜索「屏幕」→ 3" "3" "$(curl -s "$B/api/app/records?keyword=屏幕" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")"
chk "日期范围（只查今天）→ 2" "2" "$(curl -s "$B/api/app/records?from=$TODAY&to=$TODAY" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")"
chk "非法日期 → 400" "400" "$(code "$B/api/app/records?from=2026/01/01" -H "Authorization: Bearer $M1_AT")"

echo "== 4. 两个榜口径不同 =="
P=$(curl -s "$B/api/app/stats/ranking?type=points" -H "Authorization: Bearer $M1_AT")
chk "积分榜第一名是 L（50 > 12）" "组长" "$(echo "$P" | jget "['data']['rows'][0]['name']")"
chk "积分榜 L=50" "50" "$(echo "$P" | jget "['data']['rows'][0]['score']")"
S=$(curl -s "$B/api/app/stats/ranking?type=score&period=month" -H "Authorization: Bearer $M1_AT")
chk "得分榜（本月）第一名是小孩" "小孩" "$(echo "$S" | jget "['data']['rows'][0]['name']")"
chk "得分榜小孩本月 17 分" "17" "$(echo "$S" | jget "['data']['rows'][0]['period_score']")"
chk "得分榜 L 为 0（devtool 加分不算得分）" "0" "$(echo "$S" | jget "['data']['rows'][1]['period_score']")"
chk "连续打卡 3 天（11.2.3）" "3" "$(echo "$S" | jget "['data']['rows'][0]['streak']")"
chk "非法 type → 400" "400" "$(code "$B/api/app/stats/ranking?type=bad" -H "Authorization: Bearer $M1_AT")"

echo "== 5. 趋势图 =="
T=$(curl -s "$B/api/app/stats/trend?member_id=$M1_ID&period=month" -H "Authorization: Bearer $M1_AT")
chk "趋势有 3 天" "3" "$(echo "$T" | jget "['data']['points'].__len__()")"
chk "第一天净得分 4" "4" "$(echo "$T" | jget "['data']['points'][0]['net']")"
chk "普通成员查他人趋势 → 403" "403" "$(code "$B/api/app/stats/trend?member_id=1" -H "Authorization: Bearer $M1_AT")"
chk "组长查他人趋势 → 200" "200" "$(code "$B/api/app/stats/trend?member_id=$M1_ID" -H "Authorization: Bearer $L_AT")"

echo "== 6. 兑换后：积分榜降、得分榜不变 =="
curl -s -o /dev/null -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"糖果","cost":5}'
ITEM=$(curl -s $B/api/app/item/list -H "Authorization: Bearer $L_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/item/redeem -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM}"
P2=$(curl -s "$B/api/app/stats/ranking?type=points" -H "Authorization: Bearer $M1_AT")
S2=$(curl -s "$B/api/app/stats/ranking?type=score&period=month" -H "Authorization: Bearer $M1_AT")
chk "兑换后积分榜小孩 17→12" "12" "$(echo "$P2" | python -c "import sys,json;d=json.load(sys.stdin);print([r['score'] for r in d['data']['rows'] if r['name']=='小孩'][0])")"
chk "兑换后得分榜仍 17（只看赚到的）" "17" "$(echo "$S2" | python -c "import sys,json;d=json.load(sys.stdin);print([r['period_score'] for r in d['data']['rows'] if r['name']=='小孩'][0])")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
