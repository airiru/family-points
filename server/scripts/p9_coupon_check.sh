#!/usr/bin/env bash
# P9 商品券与核销投票验收（docs 9.6）
#   兑换 → 发券 → 使用 → 全员过半投票 → 核销
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
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"; }
me() { curl -s $B/api/app/me -H "Authorization: Bearer $1"; }
grant() { ( cd "$(dirname "$0")/.." && "$TOOL" -member "$1" -points "$2" ) > /dev/null 2>&1; }

echo "== 1. 造数据：组长 L + 成员 M1 / M2 =="
L=$(reg "l9$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
CODE=$(curl -s $B/api/app/family/invite-code -H "Authorization: Bearer $L_AT" | jget "['data']['invite_code']")
M1=$(reg "m91$RUN" "成员一" false); M1_AT=$(echo "$M1" | jget "['data']['token']['access_token']")
M2=$(reg "m92$RUN" "成员二" false); M2_AT=$(echo "$M2" | jget "['data']['token']['access_token']")
for t in "$M1_AT" "$M2_AT"; do
  curl -s -o /dev/null -X POST $B/api/app/family/join -H "Authorization: Bearer $t" \
    -H 'Content-Type: application/json' -d "{\"invite_code\":\"$CODE\"}"
done
grant "m91$RUN" 50
echo "  M1 余额 $(me "$M1_AT" | jget "['data']['score']")"

echo "== 2. 商品与兑换 =="
ITEM_ID=$(curl -s -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d '{"name":"冰淇淋","cost":10}' | jget "['data']['id']")
chk "建商品 → 有 id" "True" "$([ -n "$ITEM_ID" ] && echo True || echo False)"
chk "M1 兑换 → 200" "200" "$(code -X POST $B/api/app/item/redeem -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM_ID}")"
chk "兑换后余额 50→40" "40" "$(me "$M1_AT" | jget "['data']['score']")"

echo "== 3. 兑换自动发券 =="
COUPONS=$(curl -s $B/api/app/coupons -H "Authorization: Bearer $M1_AT")
CID=$(echo "$COUPONS" | jget "['data']['items'][0]['id']")
chk "券已生成" "True" "$([ -n "$CID" ] && echo True || echo False)"
chk "券状态 unused" "unused" "$(echo "$COUPONS" | jget "['data']['items'][0]['status']")"
chk "券存了商品名快照" "冰淇淋" "$(echo "$COUPONS" | jget "['data']['items'][0]['item_name']")"
chk "券存了价格快照 10" "10" "$(echo "$COUPONS" | jget "['data']['items'][0]['cost']")"

echo "== 4. 使用券（进入投票）=="
chk "他人用自己的券 → 403" "403" "$(code -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M2_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID}")"
chk "本人使用 → 200" "200" "$(code -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID}")"
chk "状态变 pending" "pending" "$(curl -s "$B/api/app/coupon/detail?coupon_id=$CID" \
  -H "Authorization: Bearer $M1_AT" | jget "['data']['coupon']['status']")"
chk "重复使用 → 409" "409" "$(code -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID}")"

echo "== 5. 投票（3 人家庭：申请人除外 2 人，需 2 票同意）=="
chk "申请人投票 → 403" "403" "$(code -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"approve\"}")"
chk "非法 vote 值 → 400" "400" "$(code -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"maybe\"}")"
V1=$(curl -s -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"approve\"}")
chk "1 票同意：仍未核销" "pending" "$(echo "$V1" | jget "['data']['status']")"
chk "重复投票 → 409" "409" "$(code -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"reject\"}")"
V2=$(curl -s -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $M2_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"approve\"}")
chk "2 票同意 → 已核销" "redeemed" "$(echo "$V2" | jget "['data']['status']")"
chk "核销后券状态 redeemed" "redeemed" "$(curl -s "$B/api/app/coupon/detail?coupon_id=$CID" \
  -H "Authorization: Bearer $M1_AT" | jget "['data']['coupon']['status']")"
chk "已核销的券不能再用 → 409" "409" "$(code -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID}")"
chk "核销不产生积分变动（仍 40）" "40" "$(me "$M1_AT" | jget "['data']['score']")"

echo "== 6. 驳回：回到 unused 且 reject_count+1 =="
chk "再兑换一张" "200" "$(code -X POST $B/api/app/item/redeem -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"item_id\":$ITEM_ID}")"
CID2=$(curl -s $B/api/app/coupons -H "Authorization: Bearer $M1_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID2}"
curl -s -o /dev/null -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID2,\"vote\":\"reject\"}"
curl -s -o /dev/null -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $M2_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID2,\"vote\":\"reject\"}"
D2=$(curl -s "$B/api/app/coupon/detail?coupon_id=$CID2" -H "Authorization: Bearer $M1_AT")
chk "反对足够 → 回到 unused" "unused" "$(echo "$D2" | jget "['data']['coupon']['status']")"
chk "reject_count = 1" "1" "$(echo "$D2" | jget "['data']['coupon']['reject_count']")"
chk "驳回后可再次使用 → 200" "200" "$(code -X POST $B/api/app/coupon/use -H "Authorization: Bearer $M1_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID2}")"

echo "== 7. 1 人家庭：无人可投，直接核销 =="
S=$(reg "s9$RUN" "独居" true); S_AT=$(echo "$S" | jget "['data']['token']['access_token']")
grant "s9$RUN" 50
SITEM=$(curl -s -X POST $B/api/app/item/save -H "Authorization: Bearer $S_AT" \
  -H 'Content-Type: application/json' -d '{"name":"独享券","cost":5}' | jget "['data']['id']")
curl -s -o /dev/null -X POST $B/api/app/item/redeem -H "Authorization: Bearer $S_AT" \
  -H 'Content-Type: application/json' -d "{\"item_id\":$SITEM}"
SCID=$(curl -s $B/api/app/coupons -H "Authorization: Bearer $S_AT" | jget "['data']['items'][0]['id']")
SU=$(curl -s -X POST $B/api/app/coupon/use -H "Authorization: Bearer $S_AT" \
  -H 'Content-Type: application/json' -d "{\"coupon_id\":$SCID}")
chk "1 人家庭直接核销" "redeemed" "$(echo "$SU" | jget "['data']['status']")"

echo "== 8. 跨组隔离 =="
chk "跨组看别人的券 → 404" "404" "$(code "$B/api/app/coupon/detail?coupon_id=$CID" \
  -H "Authorization: Bearer $S_AT")"

echo
echo "P9 商品券：通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ] || exit 1
