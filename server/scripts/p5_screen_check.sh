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
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"; }
score() { curl -s $B/api/app/me -H "Authorization: Bearer $1" | jget "['data']['score']"; }
# 上报：report <member_token> <date> <total> <night> [device_key] [model]
# 2026-11-05 起不再需要设备令牌，改用成员登录态 + 客户端自报设备标识。
report() {
  curl -s -X POST $B/api/app/screen/report -H "Authorization: Bearer $1" -H 'Content-Type: application/json' \
    -d "{\"device_key\":\"${5:-devA}\",\"device_model\":\"${6:-Pixel-Test}\",\"report_date\":\"$2\",\"total_min\":$3,\"night_min\":$4}"
}

echo "== 1. 建组时自动写入默认配置 =="
L=$(reg "l5$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']"); L_ID=$(echo "$L" | jget "['data']['member']['id']")
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

echo "== 3. 给小孩建默认规则 + 上报鉴权 =="
RULE_ID=$(echo "$RULE" | jget "['data']['rules'][0]['id']")
chk "给小孩配规则 → 200" "200" "$(code -X POST $B/api/app/screen-rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"threshold_min\":120,\"bonus_points\":2,\"penalty_points\":3,\"night_start_hour\":0,\"night_end_hour\":7,\"night_max_min\":0,\"night_rule_id\":$RULE_ID}")"
A_AT=$(admin_login "$B")
# 2026-11-05：设备令牌已删除，成员登录态即可上报
chk "未登录上报 → 401" "401" "$(code -X POST $B/api/app/screen/report -H 'Content-Type: application/json' -d '{"device_key":"x","total_min":10}')"
chk "缺 device_key → 400" "400" "$(code -X POST $B/api/app/screen/report -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"total_min":10}')"
TOKEN="$M_AT"

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

echo "== 7. 多设备：相加后结算 =="
# 同一天两台设备：A 报 30，B 报 20 → 汇总 50，按 50 判定（阈值 120 不达标，不加分）
report "$M_AT" 2026-10-25 30 0 devA 手机 >/dev/null
R2=$(report "$M_AT" 2026-10-25 20 0 devB 平板)
chk "多设备相加 = 50" "50" "$(echo "$R2" | jget "['data']['total_min']")"
chk "同日只产生 1 条结算行" "1" "$(curl -s "$B/api/app/screen/reports?member_id=$M_ID" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);r=[x for x in d['data']['reports'] if x['report_date'].startswith('2026-10-25')];print(len(r))")"

echo "== 8. 家庭屏幕详情（成员可看全组）=="
FAM=$(curl -s "$B/api/app/screen/family?date=2026-10-25" -H "Authorization: Bearer $M_AT")
chk "家庭详情 200 且含 2 人" "2" "$(echo "$FAM" | python -c "import sys,json;d=json.load(sys.stdin);print(len(d['data']['members']))")"
chk "小孩当天汇总 50" "50" "$(echo "$FAM" | python -c "import sys,json;d=json.load(sys.stdin);print([m['total_min'] for m in d['data']['members'] if m['member_id']==$M_ID][0])")"
chk "小孩有 2 台设备明细" "2" "$(echo "$FAM" | python -c "import sys,json;d=json.load(sys.stdin);print(len([m for m in d['data']['members'] if m['member_id']==$M_ID][0]['devices']))")"
chk "设备明细含型号" "True" "$(echo "$FAM" | python -c "import sys,json;d=json.load(sys.stdin);ms=[m for m in d['data']['members'] if m['member_id']==$M_ID][0]['devices'];print(any(x['device_model'] in ('手机','平板') for x in ms))")"
chk "本人设备列表 2 台" "2" "$(curl -s "$B/api/app/screen/devices" -H "Authorization: Bearer $M_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(len(d['data']['devices']))")"

echo "== 9. 后台设备原始上报明细（12.6）=="
A_ADMIN=$(admin_login "$B")
DET=$(curl -s "$B/api/admin/family/$FID/screen-device-reports?date=2026-10-25" -H "Authorization: Bearer $A_ADMIN")
chk "后台明细含小孩 2 台设备" "2" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin);print(len([r for r in d['data']['reports'] if r['member_id']==$M_ID]))")"
chk "明细保留各设备原始时长 30/20" "True" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin);v=sorted(r['total_min'] for r in d['data']['reports'] if r['member_id']==$M_ID);print(v==[20,30])")"
chk "明细带设备型号" "True" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['device_model'] in ('手机','平板') for r in d['data']['reports']))")"
chk "已登记设备 2 台" "2" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin);print(len([x for x in d['data']['devices'] if x['member_id']==$M_ID]))")"
chk "按成员筛选只返回该成员" "True" "$(curl -s "$B/api/admin/family/$FID/screen-device-reports?member_id=$M_ID" -H "Authorization: Bearer $A_ADMIN" | python -c "import sys,json;d=json.load(sys.stdin);print(all(r['member_id']==$M_ID for r in d['data']['reports']))")"
chk "日期格式非法 → 400" "400" "$(code "$B/api/admin/family/$FID/screen-device-reports?date=2026/10/25" -H "Authorization: Bearer $A_ADMIN")"

echo "== 10. 余额可为负：自动扣分无下限，主动支出仍校验（8.4）=="
# 阈值 0 / 超标扣 1000 / 不加分；不传夜间三项 → 关闭夜间判定，每次上报只产生 -1000
chk "保存高扣分规则 → 200" "200" "$(code -X POST $B/api/app/screen-rule/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$M_ID,\"threshold_min\":0,\"bonus_points\":0,\"penalty_points\":1000}")"
BAL0=$(score "$M_AT")
R10=$(report "$M_AT" 2026-11-01 200 1 devA 手机)
chk "超标上报只扣日总量 -1000" "-1000" "$(echo "$R10" | jget "['data']['day_points']")"
BAL1=$(score "$M_AT")
chk "第一次扣分 = 原 -1000" "$((BAL0-1000))" "$BAL1"
chk "余额已扣成负数" "True" "$([ "$BAL1" -lt 0 ] && echo True || echo False)"
R11=$(report "$M_AT" 2026-11-02 200 1 devA 手机)
chk "负余额后第二次仍扣 -1000" "-1000" "$(echo "$R11" | jget "['data']['day_points']")"
BAL2=$(score "$M_AT")
chk "负余额后继续扣：新余额 = 旧 -1000" "$((BAL1-1000))" "$BAL2"
NEG_REC=$(curl -s "$B/api/app/records?limit=200" -H "Authorization: Bearer $M_AT")
chk "两条负分流水都落库" "2" "$(echo "$NEG_REC" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['source']=='screen' and r['points']==-1000))")"
chk "负余额时赠送 → 400" "400" "$(code -X POST $B/api/app/transfer/send -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$L_ID,\"amount\":1}")"
chk "拒绝后余额不变" "$BAL2" "$(score "$M_AT")"

echo "== 11. 逐小时明细 + 成员详情（2026-11-06）=="
# 快照：只落明细，不结算、不发分
BAL_SNAP=$(score "$M_AT")
HOURS='[0,0,0,0,0,0,0,0,0,10,20,0,0,0,0,0,0,0,0,30,40,0,0,0]'
chk "今日快照 → 200" "200" "$(code -X POST $B/api/app/screen/snapshot -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"device_key\":\"devA\",\"device_model\":\"手机\",\"report_date\":\"2026-11-03\",\"total_min\":100,\"night_min\":0,\"hourly\":$HOURS}")"
chk "快照不改余额（不结算）" "$BAL_SNAP" "$(score "$M_AT")"
chk "快照桶数不是 24 → 400" "400" "$(code -X POST $B/api/app/screen/snapshot -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{\"device_key\":\"devA\",\"report_date\":\"2026-11-03\",\"hourly\":[1,2]}')"
chk "上报带非 24 桶 → 400" "400" "$(code -X POST $B/api/app/screen/report -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{\"device_key\":\"devA\",\"report_date\":\"2026-11-03\",\"total_min\":5,\"hourly\":[1,2]}')"

DET=$(curl -s "$B/api/app/screen/member/$M_ID?date=2026-11-03" -H "Authorization: Bearer $M_AT")
chk "成员详情 200" "True" "$(echo "$DET" | python -c "import sys,json;print(json.load(sys.stdin)['ok'])")"
chk "详情 total = 桶之和 100" "100" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin)['data'];print(d['total_min'])")"
chk "详情 hourly 24 桶" "24" "$(echo "$DET" | python -c "import sys,json;print(len(json.load(sys.stdin)['data']['hourly']))")"
chk "详情 hourly 与上传一致" "True" "$(echo "$DET" | python -c "import sys,json;print(json.load(sys.stdin)['data']['hourly']==$HOURS)")"
chk "详情近 7 天固定 7 条" "7" "$(echo "$DET" | python -c "import sys,json;print(len(json.load(sys.stdin)['data']['days']))")"
chk "详情含设备小时明细" "True" "$(echo "$DET" | python -c "import sys,json;d=json.load(sys.stdin)['data'];print(any(len(x['hourly'])==24 for x in d['devices']))")"
chk "详情 is_self = true" "True" "$(echo "$DET" | python -c "import sys,json;print(json.load(sys.stdin)['data']['is_self'])")"

# 组长看别人
OTHER=$(curl -s "$B/api/app/screen/member/$M_ID?date=2026-11-03" -H "Authorization: Bearer $L_AT")
chk "组长可看成员详情" "True" "$(echo "$OTHER" | python -c "import sys,json;print(json.load(sys.stdin)['ok'])")"
chk "组长看到的是他人（is_self=false）" "False" "$(echo "$OTHER" | python -c "import sys,json;print(json.load(sys.stdin)['data']['is_self'])")"
chk "不存在的成员 → 404" "404" "$(code "$B/api/app/screen/member/999999" -H "Authorization: Bearer $L_AT")"
chk "非法 member_id → 400" "400" "$(code "$B/api/app/screen/member/abc" -H "Authorization: Bearer $L_AT")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
