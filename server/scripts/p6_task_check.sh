#!/usr/bin/env bash
# P6 任务与投票验收（冻结 → 接取 → 提交 → 全员投票过半 → 转移）
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
me() { curl -s $B/api/app/me -H "Authorization: Bearer $1"; }
score() { me "$1" | jget "['data']['score']"; }
frozen() { me "$1" | jget "['data']['frozen_score']"; }
grant() { ( cd "$(dirname "$0")/.." && "$TOOL" -member "$1" -points "$2" ) > /dev/null 2>&1; }

echo "== 1. 造数据：组长 L + 成员 M1 / M2 =="
L=$(reg "l6$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
FID=$(echo "$L" | jget "['data']['member']['family_id']")
M1=$(reg "m61$RUN" "成员一" false); M1_AT=$(echo "$M1" | jget "['data']['token']['access_token']")
M2=$(reg "m62$RUN" "成员二" false); M2_AT=$(echo "$M2" | jget "['data']['token']['access_token']")
A_AT=$(admin_login "$B")
for u in "m61$RUN" "m62$RUN"; do curl -s -o /dev/null -X POST $B/api/admin/family/$FID/invite -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"username\":\"$u\"}"; done
grant "l6$RUN" 100
echo "  L 余额 $(score "$L_AT")，M1=$(score "$M1_AT")，M2=$(score "$M2_AT")"

echo "== 2. 发布任务：冻结积分 =="
chk "分值 0 → 400" "400" "$(code -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"倒垃圾","points":0}')"
chk "余额不足 → 400" "400" "$(code -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"倒垃圾","points":9999}')"
T1=$(curl -s -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"倒垃圾","points":30}')
T1_ID=$(echo "$T1" | jget "['data']['id']")
chk "发布成功 → 200" "True" "$([ -n "$T1_ID" ] && echo True || echo False)"
chk "发布后可用余额 100→70" "70" "$(score "$L_AT")"
chk "发布后冻结 30" "30" "$(frozen "$L_AT")"
chk "发布写了一条 0 分说明流水" "0" "$(curl -s "$B/api/app/records?limit=1" -H "Authorization: Bearer $L_AT" | jget "['data']['items'][0]['points']")"

echo "== 3. 接取规则 =="
chk "发布者接自己 → 403" "403" "$(code -X POST $B/api/app/task/claim -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID}")"
chk "M1 接取 → 200" "200" "$(code -X POST $B/api/app/task/claim -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID}")"
chk "M2 再接 → 409" "409" "$(code -X POST $B/api/app/task/claim -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID}")"
chk "M1 放弃 → 200" "200" "$(code -X POST $B/api/app/task/abandon -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID}")"
chk "M2 接取 → 200" "200" "$(code -X POST $B/api/app/task/claim -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID}")"

echo "== 4. 提交（说明必填）=="
chk "非接取者提交 → 403" "403" "$(code -X POST $B/api/app/task/submit -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID,\"note\":\"我做了\"}")"
chk "说明为空 → 400" "400" "$(code -X POST $B/api/app/task/submit -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID,\"note\":\"  \"}")"
SUB=$(curl -s -X POST $B/api/app/task/submit -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID,\"note\":\"倒完了\"}")
SUB_ID=$(echo "$SUB" | jget "['data']['id']")
chk "接取者提交 → 200" "True" "$([ -n "$SUB_ID" ] && echo True || echo False)"
chk "重复提交 → 409" "409" "$(code -X POST $B/api/app/task/submit -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T1_ID,\"note\":\"again\"}")"

echo "== 5. 投票过半才转移（3 人家庭：发布者除外 2 票，需 2 票同意）=="
chk "发布者投票 → 403" "403" "$(code -X POST $B/api/app/task/vote -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB_ID,\"vote\":\"approve\"}")"
V1=$(curl -s -X POST $B/api/app/task/vote -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB_ID,\"vote\":\"approve\"}")
chk "1 票同意：仍未通过" "submitted" "$(echo "$V1" | jget "['data']['status']")"
chk "1 票时未转移（L 冻结仍 30）" "30" "$(frozen "$L_AT")"
chk "重复投票 → 409" "409" "$(code -X POST $B/api/app/task/vote -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB_ID,\"vote\":\"reject\"}")"
V2=$(curl -s -X POST $B/api/app/task/vote -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB_ID,\"vote\":\"approve\"}")
chk "2 票同意：通过" "approved" "$(echo "$V2" | jget "['data']['status']")"
chk "通过即转移 → L 冻结归零" "0" "$(frozen "$L_AT")"
chk "完成者 M2 得 30 分" "30" "$(score "$M2_AT")"
chk "发布者 L 仍 70 分（只减了冻结）" "70" "$(score "$L_AT")"
REC=$(curl -s "$B/api/app/records?limit=20" -H "Authorization: Bearer $L_AT")
chk "任务转移产生 -30 与 +30 两条流水" "-30,30" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(','.join(str(r['points']) for r in sorted([x for x in d['data']['items'] if x['source']=='task' and x['points']!=0], key=lambda x:x['points'])))")"
chk "转移流水合计为 0（家庭总积分不变）" "0" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(r['points'] for r in d['data']['items'] if r['source']=='task'))")"

echo "== 6. 关闭任务：冻结退回 =="
T2=$(curl -s -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"洗碗","points":20}')
T2_ID=$(echo "$T2" | jget "['data']['id']")
chk "再发布后可用 50 / 冻结 20" "50/20" "$(score "$L_AT")/$(frozen "$L_AT")"
chk "非发布者关闭 → 403" "403" "$(code -X POST $B/api/app/task/close -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T2_ID}")"
chk "发布者关闭 → 200" "200" "$(code -X POST $B/api/app/task/close -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T2_ID}")"
chk "退回后可用 70 / 冻结 0" "70/0" "$(score "$L_AT")/$(frozen "$L_AT")"

echo "== 7. 驳回：提前否决 + 重提 3 次上限 =="
T3=$(curl -s -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"背单词","points":10}')
T3_ID=$(echo "$T3" | jget "['data']['id']")
curl -s -o /dev/null -X POST $B/api/app/task/claim -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T3_ID}"
for i in 1 2 3; do
  S=$(curl -s -X POST $B/api/app/task/submit -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T3_ID,\"note\":\"第 $i 次\"}")
  SID=$(echo "$S" | jget "['data']['id']")
  R=$(curl -s -X POST $B/api/app/task/vote -H "Authorization: Bearer $M2_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SID,\"vote\":\"reject\"}")
  chk "第 $i 次：一张反对票即提前驳回" "rejected" "$(echo "$R" | jget "['data']['status']")"
done
chk "第 4 次提交 → 409（超 3 次上限）" "409" "$(code -X POST $B/api/app/task/submit -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$T3_ID,\"note\":\"第 4 次\"}")"

echo "== 8. 单成员家庭禁止发布任务 =="
S1=$(reg "solo$RUN" "独居" true); S1_AT=$(echo "$S1" | jget "['data']['token']['access_token']")
chk "只有 1 人 → 400" "400" "$(code -X POST $B/api/app/task/create -H "Authorization: Bearer $S1_AT" -H 'Content-Type: application/json' -d '{"title":"x","points":1}')"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
