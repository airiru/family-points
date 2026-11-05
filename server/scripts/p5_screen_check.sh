#!/usr/bin/env bash
# P5 睡觉规则 + 屏幕时间结算验收
set -u
. "$(dirname "$0")/lib_admin.sh"
B=http://127.0.0.1:8080
RUN=$(date +%s)
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
reg() { curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"name\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"; }
score() { curl -s $B/api/app/me -H "Authorization: Bearer $1" | jget "['data']['score']"; }
# 上报：report <token> <date> <total> <night>
report() {
  curl -s -X POST $B/api/device/screen/report -H "X-Device-Token: $1" -H 'Content-Type: application/json' \
    -d "{\"report_date\":\"$2\",\"total_min\":$3,\"night_min\":$4}"
}

echo "== 1. 建组时自动写入默认配置 =="
L=$(reg "l5$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
CODE=$(curl -s $B/api/app/family/invite-code -H "Authorization: Bearer $L_AT" | jget "['data']['invite_code']")
M=$(reg "m5$RUN" "小孩" false); M_AT=$(echo "$M" | jget "['data']['token']['access_token']"); M_ID=$(echo "$M" | jget "['data']['member']['id']")
curl -s -o /dev/null -X POST $B/api/app/family/join -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"invite_code\":\"$CODE\"}"
FID=$(curl -s $B/api/app/family -H "Authorization: Bearer $L_AT" | jget "['data']['id']")

RULE=$(curl -s $B/api/app/rule -H "Authorization: Bearer $L_AT")
chk "默认规则存在" "按时睡觉" "$(echo "$RULE" | jget "['data']['rules'][0]['name']")"
chk "默认达标分=2" "2" "$(echo "$RULE" | jget "['data']['rules'][0]['points']")"
echo "  默认档位：$(echo "$RULE" | jget "['data']['rules'][0]['streaks']")"
SR=$(curl -s $B/api/app/screen-rule -H "Authorization: Bearer $L_AT")
chk "组长默认阈值 120" "120" "$(echo "$SR" | jget "['data']['screen_rules'][0]['threshold_min']")"
chk "默认夜间 0 点开始" "0" "$(echo "$SR" | jget "['data']['screen_rules'][0]['night_start_hour']")"
chk "默认夜间 7 点结束" "7" "$(echo "$SR" | jget "['data']['screen_rules'][0]['night_end_hour']")"
chk "默认夜间上限 0 分钟" "0" "$(echo "$SR" | jget "['data']['screen_rules'][0]['night_max_min']")"

echo "== 2. 配置校验 =="
chk "非组长改规则 → 403" "403" "$(code -X POST $B/api/app/rule/save -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"name":"x","points":1,"streaks":[]}')"
chk "档位 every=0 → 400" "400" "$(code -X POST $B/api/app/rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"x","points":1,"streaks":[{"every":0,"bonus":5}]}')"
chk "档位重复 → 400" "400" "$(code -X POST $B/api/app/rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"x","points":1,"streaks":[{"every":3,"bonus":5},{"every":3,"bonus":7}]}')"
chk "夜间三项不全 → 400" "400" "$(code -X POST $B/api/app/screen-rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"threshold_min\":120,\"bonus_points\":2,\"penalty_points\":3,\"night_start_hour\":0}")"
chk "开始=结束 → 400" "400" "$(code -X POST $B/api/app/screen-rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"threshold_min\":120,\"bonus_points\":2,\"penalty_points\":3,\"night_start_hour\":7,\"night_end_hour\":7,\"night_max_min\":0,\"night_rule_id\":1}")"

echo "== 3. 给小孩建默认规则 + 设备令牌 =="
RULE_ID=$(echo "$RULE" | jget "['data']['rules'][0]['id']")
chk "给小孩配规则 → 200" "200" "$(code -X POST $B/api/app/screen-rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"threshold_min\":120,\"bonus_points\":2,\"penalty_points\":3,\"night_start_hour\":0,\"night_end_hour\":7,\"night_max_min\":0,\"night_rule_id\":$RULE_ID}")"
A_AT=$(admin_login "$B")
DEV=$(curl -s -X POST $B/api/admin/family/$FID/device -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"name\":\"小孩手机\"}")
TOKEN=$(echo "$DEV" | jget "['data']['token']")
DEV_ID=$(echo "$DEV" | jget "['data']['device_id']")
chk "设备令牌已生成" "True" "$([ ${#TOKEN} -eq 64 ] && echo True || echo False)"
chk "无令牌上报 → 401" "401" "$(code -X POST $B/api/device/screen/report -H 'Content-Type: application/json' -d '{"total_min":10}')"
chk "错误令牌上报 → 401" "401" "$(code -X POST $B/api/device/screen/report -H 'X-Device-Token: deadbeef' -H 'Content-Type: application/json' -d '{"total_min":10}')"

echo "== 4. 连续 4 天上报（含第 3 天档位奖励）=="
R1=$(report "$TOKEN" 2026-10-01 60 0); chk "第1天 日总量 +2" "2" "$(echo "$R1" | jget "['data']['day_points']")"
chk "第1天 夜间达标" "True" "$(echo "$R1" | jget "['data']['night_ok']")"
chk "第1天 连续 1 天" "1" "$(echo "$R1" | jget "['data']['streak_days']")"
chk "第1天 累计 4 分" "4" "$(score "$M_AT")"
report "$TOKEN" 2026-10-02 60 0 > /dev/null
chk "第2天 累计 8 分" "8" "$(score "$M_AT")"
R3=$(report "$TOKEN" 2026-10-03 60 0)
chk "第3天 连续 3 天" "3" "$(echo "$R3" | jget "['data']['streak_days']")"
chk "第3天 命中档位奖励 +5" "5" "$(echo "$R3" | jget "['data']['awards'][0]['bonus']")"
chk "第3天 累计 17 分（+2 日 +2 夜 +5 档位）" "17" "$(score "$M_AT")"
R4=$(report "$TOKEN" 2026-10-04 60 45)
chk "第4天 夜间未达标" "False" "$(echo "$R4" | jget "['data']['night_ok']")"
chk "第4天 连续清零" "0" "$(echo "$R4" | jget "['data']['streak_days']")"
chk "第4天 累计 19 分（只加日总量 2）" "19" "$(score "$M_AT")"

echo "== 5. 后报覆盖：同一天以最后一次为准 =="
BEFORE=$(score "$M_AT")
report "$TOKEN" 2026-10-20 200 45 > /dev/null
MID=$(score "$M_AT")
chk "先报 200 分钟 → 扣 3 分" "$((BEFORE-3))" "$MID"
R=$(report "$TOKEN" 2026-10-20 60 0)
chk "覆盖为 60 分钟 → 日总量转 +2" "2" "$(echo "$R" | jget "['data']['day_points']")"
chk "覆盖后总分 = 原 +4（回滚 -3，重算 +4）" "$((BEFORE+4))" "$(score "$M_AT")"
R2=$(report "$TOKEN" 2026-10-20 60 0)
chk "数值未变 → changed=false" "False" "$(echo "$R2" | jget "['data']['changed']")"

echo "== 6. 流水与快照 =="
REC=$(curl -s "$B/api/app/records?limit=200" -H "Authorization: Bearer $M_AT")
chk "存在 source=screen 记录" "True" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['source']=='screen' for r in d['data']['items']))")"
chk "存在 source=night 记录" "True" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['source']=='night' for r in d['data']['items']))")"
chk "第3天记录带 streak_award" "True" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r.get('streak_award') for r in d['data']['items']))" 2>/dev/null || echo False)"
chk "覆盖后 10-20 只有 1 条结算行（一天一行）" "1" "$(curl -s "$B/api/app/screen/reports?member_id=$M_ID" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);r=[x for x in d['data']['reports'] if x['report_date'].startswith('2026-10-20')];print(len(r))")"
chk "覆盖后 10-20 的流水只有 2 条（日+夜，旧的已回滚）" "2" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['occurred_at'].startswith('2026-10-20')))")"

echo "== 7. 吊销设备后上报失败 =="
chk "吊销设备 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/device/revoke -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"device_id\":$DEV_ID}")"
chk "吊销后上报 → 401" "401" "$(code -X POST $B/api/device/screen/report -H "X-Device-Token: $TOKEN" -H 'Content-Type: application/json' -d '{"total_min":10}')"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
