#!/usr/bin/env bash
# P11 站内通知 + 个人资料验收（对应需求：投票要有通知、可改名称头像）
set -u
. "$(dirname "$0")/lib_admin.sh"
B=http://127.0.0.1:8080
RUN=$(date +%s)
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
reg() { curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
  -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"通知组$RUN\"}"; }
# notif <token> <type> → 该类型未读通知条数
notif() {
  curl -s "$B/api/app/notification/list?limit=50" -H "Authorization: Bearer $1" | python -c "
import sys,json
d=json.load(sys.stdin)
items=d['data']['items']
print(sum(1 for x in items if x['type']=='$2'))" 2>/dev/null
}

echo "== 0. 建组：组长 L + 成员 A + 成员 B =="
L=$(reg "nl$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
FID=$(echo "$L" | jget "['data']['member']['family_id']")
A=$(reg "na$RUN" "甲" false); A_AT=$(echo "$A" | jget "['data']['token']['access_token']"); A_ID=$(echo "$A" | jget "['data']['member']['id']")
Bm=$(reg "nb$RUN" "乙" false); B_AT=$(echo "$Bm" | jget "['data']['token']['access_token']"); B_ID=$(echo "$Bm" | jget "['data']['member']['id']")
ADMIN_AT=$(admin_login "$B")
for u in "na$RUN" "nb$RUN"; do
  curl -s -o /dev/null -X POST $B/api/admin/family/$FID/invite -H "Authorization: Bearer $ADMIN_AT" -H 'Content-Type: application/json' -d "{\"username\":\"$u\"}"
done
chk "三人同组" "3" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $L_AT" | python -c "import sys,json;print(len(json.load(sys.stdin)['data']['members']))")"

# 造一点积分：任务发布要冻结、赠送要有余额（走 devtool，仅测试用）
DEVTOOL=./devtool.exe
[ -x "$DEVTOOL" ] || (go build -o devtool.exe ./cmd/devtool && chmod +x devtool.exe) 2>/dev/null
$DEVTOOL -member "nl$RUN" -points 100 >/dev/null 2>&1 || true
$DEVTOOL -member "na$RUN" -points 50 >/dev/null 2>&1 || true

echo "== 1. 个人资料：改名称 + 头像 =="
chk "改昵称 200" "200" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{"nickname":"甲改名","avatar":"🐻"}')"
ME=$(curl -s $B/api/app/me -H "Authorization: Bearer $A_AT")
chk "昵称已更新" "甲改名" "$(echo "$ME" | jget "['data']['nickname']")"
chk "头像已更新" "🐻" "$(echo "$ME" | jget "['data']['avatar']")"
chk "空昵称+空头像 → 400" "400" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{}')"
chk "昵称超 32 字 → 400" "400" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"nickname\":\"$(python -c 'print("字"*33)')\"}")"
chk "资料接口带 username → 400" "400" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{"nickname":"不改","username":"hacker"}')"
chk "username 未被改写" "na$RUN" "$(curl -s $B/api/app/me -H "Authorization: Bearer $A_AT" | jget "['data']['username']")"
chk "被拒请求也没有改掉昵称" "甲改名" "$(curl -s $B/api/app/me -H "Authorization: Bearer $A_AT" | jget "['data']['nickname']")"

echo "== 2. 赠送到账通知 =="
curl -s -o /dev/null -X POST $B/api/app/transfer/send -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"to_member_id\":$A_ID,\"amount\":5,\"note\":\"测试\"}"
chk "受赠方收到 transfer_in" "1" "$(notif "$A_AT" transfer_in)"
chk "赠送方自己不收" "0" "$(notif "$L_AT" transfer_in)"
chk "赠送不重复（旧 transfer = 0）" "0" "$(notif "$A_AT" transfer)"

echo "== 3. 任务：提交 → 待投票通知；通过 → 结果通知 =="
TID=$(curl -s -X POST $B/api/app/task/create -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"title":"倒垃圾","points":10}' | jget "['data']['id']")
curl -s -o /dev/null -X POST $B/api/app/task/claim -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$TID}"
curl -s -o /dev/null -X POST $B/api/app/task/submit -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"task_id\":$TID,\"note\":\"做完了\"}"
chk "发布者收到 task_submitted" "1" "$(notif "$L_AT" task_submitted)"
chk "另一成员收到 task_submitted" "1" "$(notif "$B_AT" task_submitted)"
chk "提交者自己不收" "0" "$(notif "$A_AT" task_submitted)"

SUB=$(curl -s "$B/api/app/task/detail?task_id=$TID" -H "Authorization: Bearer $L_AT" | jget "['data']['submissions'][0]['id']")
# 有投票权 = 成员数 − 发布者 = 2（乙 + 甲），发布者不能投
curl -s -o /dev/null -X POST $B/api/app/task/vote -H "Authorization: Bearer $B_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB,\"vote\":\"approve\"}"
curl -s -o /dev/null -X POST $B/api/app/task/vote -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"submission_id\":$SUB,\"vote\":\"approve\"}"
chk "完成者收到 task_decided" "1" "$(notif "$A_AT" task_decided)"
chk "任务结果不重复（旧 task_review = 0）" "0" "$(notif "$A_AT" task_review)"

echo "== 4. 券核销：发起 → 待投票通知；通过 → 结果通知 =="
curl -s -o /dev/null -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"冰淇淋","cost":1}'
IID=$(curl -s $B/api/app/item/list -H "Authorization: Bearer $A_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/item/redeem -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$IID}"
chk "兑换者收到 redeem_success" "1" "$(notif "$A_AT" redeem_success)"
chk "组长不收兑换成功通知" "0" "$(notif "$L_AT" redeem_success)"
chk "另一成员不收兑换成功通知" "0" "$(notif "$B_AT" redeem_success)"
CID=$(curl -s $B/api/app/coupons -H "Authorization: Bearer $A_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/coupon/use -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID}"
chk "组长收到 coupon_pending" "1" "$(notif "$L_AT" coupon_pending)"
chk "另一成员收到 coupon_pending" "1" "$(notif "$B_AT" coupon_pending)"
chk "申请人自己不收" "0" "$(notif "$A_AT" coupon_pending)"

curl -s -o /dev/null -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"approve\"}"
curl -s -o /dev/null -X POST $B/api/app/coupon/vote -H "Authorization: Bearer $B_AT" -H 'Content-Type: application/json' -d "{\"coupon_id\":$CID,\"vote\":\"approve\"}"
chk "申请人收到 coupon_decided" "1" "$(notif "$A_AT" coupon_decided)"

echo "== 4b. 失败兑换不产生通知 =="
curl -s -o /dev/null -X POST $B/api/app/item/save -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"超贵","cost":99999}'
EXPENSIVE=$(curl -s $B/api/app/item/list -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);print([x['id'] for x in d['data']['items'] if x['cost']==99999][0])")
chk "余额不足兑换 → 400" "400" "$(code -X POST $B/api/app/item/redeem -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"item_id\":$EXPENSIVE}")"
chk "失败兑换后 redeem_success 仍为 1" "1" "$(notif "$A_AT" redeem_success)"

echo "== 5. 通知已读 =="
NID=$(curl -s "$B/api/app/notification/list?limit=1" -H "Authorization: Bearer $A_AT" | jget "['data']['items'][0]['id']")
chk "标记已读 200" "200" "$(code -X POST $B/api/app/notification/read -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"id\":$NID}")"
chk "未读列表少一条" "False" "$(curl -s "$B/api/app/notification/list?unread=1&limit=50" -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(x['id']==$NID for x in d['data']['items']))")"

echo
echo "P11 通知与资料：通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ] || exit 1
