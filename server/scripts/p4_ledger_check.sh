#!/usr/bin/env bash
# P4 账目核心验收（所有积分变动都要留流水）
# 依赖：/tmp/devtool（测试脚手架，只改余额不写流水，用于准备初始余额）
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
score() { curl -s $B/api/app/me -H "Authorization: Bearer $1" | jget "['data']['score']"; }
grant() { ( cd "$(dirname "$0")/.." && "$TOOL" -member "$1" -points "$2" ) > /dev/null 2>&1; }

echo "== 1. 造数据：组长 L + 成员 M1 / M2 =="
L=$(reg "l4$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
CODE=$(curl -s $B/api/app/family/invite-code -H "Authorization: Bearer $L_AT" | jget "['data']['invite_code']")
M1=$(reg "m41$RUN" "成员一" false); M1_AT=$(echo "$M1" | jget "['data']['token']['access_token']"); M1_ID=$(echo "$M1" | jget "['data']['member']['id']")
M2=$(reg "m42$RUN" "成员二" false); M2_AT=$(echo "$M2" | jget "['data']['token']['access_token']"); M2_ID=$(echo "$M2" | jget "['data']['member']['id']")
for t in "$M1_AT" "$M2_AT"; do curl -s -o /dev/null -X POST $B/api/app/family/join -H "Authorization: Bearer $t" -H 'Content-Type: application/json' -d "{\"invite_code\":\"$CODE\"}"; done
echo "  L/M1/M2 已就位，组邀请码=$CODE"

echo "== 2. 准备余额（devtool 脚手架，不写流水）=="
grant "m41$RUN" 50
grant "m42$RUN" 0
grant "m42$RUN" 20
chk "M1 余额=50" "50" "$(score "$M1_AT")"
chk "M2 余额=20" "20" "$(score "$M2_AT")"

echo "== 3. 商品管理（组长）=="
chk "非组长建商品 → 403" "403" "$(code -X POST $B/api/app/item/save -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d '{"name":"x","cost":1}')"
chk "组长建商品 → 200" "200" "$(code -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"冰淇淋","cost":30}')"
ITEM_ID=$(curl -s $B/api/app/item/list -H "Authorization: Bearer $L_AT" | jget "['data']['items'][0]['id']")
chk "商品列表含它" "冰淇淋" "$(curl -s $B/api/app/item/list -H "Authorization: Bearer $L_AT" | jget "['data']['items'][0]['name']")"

echo "== 4. 兑换：扣分 + 写负分流水 =="
chk "兑换成功 → 200" "200" "$(code -X POST $B/api/app/item/redeem -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM_ID}")"
chk "M1 余额 50→20" "20" "$(score "$M1_AT")"
REC=$(curl -s "$B/api/app/records?limit=5" -H "Authorization: Bearer $M1_AT")
chk "流水出现兑换记录" "-30" "$(echo "$REC" | jget "['data']['items'][0]['points']")"
chk "流水 source=redeem" "redeem" "$(echo "$REC" | jget "['data']['items'][0]['source']")"
RID=$(echo "$REC" | jget "['data']['items'][0]['redemption_id']")

echo "== 5. 兑换：余额不足必须拒绝，且不产生流水 =="
BEFORE=$(echo "$REC" | jget "['data']['total']")
chk "余额不足 → 400" "400" "$(code -X POST $B/api/app/item/redeem -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM_ID}")"
AFTER=$(curl -s "$B/api/app/records?limit=1" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")
chk "失败不写流水（总数不变）" "$BEFORE" "$AFTER"
chk "M1 余额未被扣" "20" "$(score "$M1_AT")"

echo "== 6. 赠送：两条流水、总积分守恒 =="
chk "赠送自己 → 400" "400" "$(code -X POST $B/api/app/transfer/send -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$M1_ID,\"amount\":5}")"
chk "金额 0 → 400" "400" "$(code -X POST $B/api/app/transfer/send -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$M2_ID,\"amount\":0}")"
chk "余额不足 → 400" "400" "$(code -X POST $B/api/app/transfer/send -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$M2_ID,\"amount\":999}")"
SUM_BEFORE=$(( $(score "$M1_AT") + $(score "$M2_AT") ))
chk "赠送 10 分 → 200" "200" "$(code -X POST $B/api/app/transfer/send -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$M2_ID,\"amount\":10,\"note\":\"买书\"}")"
chk "M1 20→10" "10" "$(score "$M1_AT")"
chk "M2 20→30" "30" "$(score "$M2_AT")"
SUM_AFTER=$(( $(score "$M1_AT") + $(score "$M2_AT") ))
chk "家庭总积分守恒" "$SUM_BEFORE" "$SUM_AFTER"
TID=$(curl -s "$B/api/app/transfer/list" -H "Authorization: Bearer $M1_AT" | jget "['data']['transfers'][0]['id']")
TCOUNT=$(curl -s "$B/api/app/records?limit=200" -H "Authorization: Bearer $M1_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['transfer_id']==$TID))")
chk "一次赠送产生 2 条流水" "2" "$TCOUNT"

echo "== 7. 撤销（仅管理员）=="
chk "成员撤销兑换 → 401" "401" "$(code -X POST $B/api/admin/family/1/item/redeem/revoke -H "Authorization: Bearer $M1_AT" -H 'Content-Type: application/json' -d "{\"redemption_id\":$RID}")"
A_AT=$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d '{"username":"admins","password":"admins123"}' | jget "['data']['token']['access_token']")
FID=$(curl -s $B/api/app/family -H "Authorization: Bearer $L_AT" | jget "['data']['id']")
chk "管理员撤销兑换 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/item/redeem/revoke -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"redemption_id\":$RID}")"
chk "M1 余额 10→40（退回 30）" "40" "$(score "$M1_AT")"
chk "重复撤销 → 409" "409" "$(code -X POST $B/api/admin/family/$FID/item/redeem/revoke -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"redemption_id\":$RID}")"
chk "撤销后流水 +30 且 source=redeem" "30" "$(curl -s "$B/api/app/records?limit=1" -H "Authorization: Bearer $M1_AT" | jget "['data']['items'][0]['points']")"
chk "管理员撤销赠送 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/transfer/revoke -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"transfer_id\":$TID}")"
chk "M1 收回 10 分 → 50" "50" "$(score "$M1_AT")"
chk "M2 退回 10 分 → 20" "20" "$(score "$M2_AT")"
chk "重复撤销赠送 → 409" "409" "$(code -X POST $B/api/admin/family/$FID/transfer/revoke -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"transfer_id\":$TID}")"

echo "== 8. 流水完整性：每条都在，且不删原记录 =="
TOTAL=$(curl -s "$B/api/app/records?limit=200" -H "Authorization: Bearer $M1_AT" | jget "['data']['total']")
echo "  M1 可见流水共 $TOTAL 条"
chk "流水 ≥5 条（兑换1 + 赠送2 + 撤销2）" "True" "$([ "$TOTAL" -ge 5 ] && echo True || echo False)"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
