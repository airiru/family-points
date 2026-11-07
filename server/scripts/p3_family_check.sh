#!/usr/bin/env bash
# P3 家庭组与成员验收（邀请码 / 加入 / 退出 / 踢出 / 组长转让 / 隐藏 / 改名）
# 幂等：每次用带时间戳的用户名。
set -u
B=http://127.0.0.1:8080
RUN=$(date +%s)
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
reg() { # reg <用户名> <昵称> <是否建组>
  curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"组$RUN\"}"
}

echo "== 1. 造数据：组长 L + 待入组 M =="
L=$(reg "l$RUN" "组长甲" true)
L_AT=$(echo "$L" | jget "['data']['token']['access_token']")
L_ID=$(echo "$L" | jget "['data']['member']['id']")
FID=$(echo "$L" | jget "['data']['member']['family_id']")
M=$(reg "m$RUN" "成员乙" false)
M_AT=$(echo "$M" | jget "['data']['token']['access_token']")
M_ID=$(echo "$M" | jget "['data']['member']['id']")
echo "  组长 id=$L_ID  成员 id=$M_ID  组 family_id=$FID"

echo "== 2. 家庭码：唯一、不可重置 =="
CODE=$(curl -s $B/api/app/family/code -H "Authorization: Bearer $L_AT" | jget "['data']['code']")
HID=$(curl -s $B/api/app/family/code -H "Authorization: Bearer $L_AT" | jget "['data']['hidden']")
echo "  家庭码=$CODE hidden=$HID"
chk "家庭码长度 8" "8" "${#CODE}"
chk "再次查询家庭码不变（不可重置）" "$CODE" "$(curl -s $B/api/app/family/code -H "Authorization: Bearer $L_AT" | jget "['data']['code']")"
chk "未入组者查组内家庭码 → 403" "403" "$(code $B/api/app/family/code -H "Authorization: Bearer $M_AT")"

echo "== 3. 搜索与申请审批（凭码加入必须过组长） =="
chk "错误码搜索 → found=false" "False" "$(curl -s "$B/api/app/family/search?code=ZZZZZZZZ" -H "Authorization: Bearer $M_AT" | jget "['data']['found']")"
SR=$(curl -s "$B/api/app/family/search?code=$CODE" -H "Authorization: Bearer $M_AT")
chk "正确码搜索 → found=true" "True" "$(echo "$SR" | jget "['data']['found']")"
chk "搜索返回组名" "组$RUN" "$(echo "$SR" | jget "['data']['name']")"
chk "凭码申请 → 200" "200" "$(code -X POST $B/api/app/family/request -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}")"
RQ=$(curl -s $B/api/app/family/requests -H "Authorization: Bearer $L_AT")
RQ_ID=$(echo "$RQ" | jget "['data']['items'][0]['id']")
chk "组长看到待审批申请" "True" "$([ "$RQ_ID" != "None" ] && [ -n "$RQ_ID" ] && echo True || echo False)"
chk "组长审批通过 → 200" "200" "$(code -X POST $B/api/app/family/request/decide -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$RQ_ID,\"accept\":true}")"
chk "加入后 role=member" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $M_AT" | jget "['data']['role']")"
chk "已入组再申请 → 409" "409" "$(code -X POST $B/api/app/family/request -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}")"

echo "== 4. 隐藏 = 搜不到 + 凭码申请也拒绝；随后 N 正常入组 =="
curl -s -o /dev/null -X POST $B/api/app/family/code/hidden -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"hidden":true}'
N=$(reg "n$RUN" "成员丙" false)
N_AT=$(echo "$N" | jget "['data']['token']['access_token']")
N_ID=$(echo "$N" | jget "['data']['member']['id']")
chk "隐藏后搜索 → found=false" "False" "$(curl -s "$B/api/app/family/search?code=$CODE" -H "Authorization: Bearer $N_AT" | jget "['data']['found']")"
chk "隐藏后凭码申请 → 400（等价于不存在）" "400" "$(code -X POST $B/api/app/family/request -H "Authorization: Bearer $N_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}")"
curl -s -o /dev/null -X POST $B/api/app/family/code/hidden -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"hidden":false}'
curl -s -o /dev/null -X POST $B/api/app/family/request -H "Authorization: Bearer $N_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}"
chk "申请后仍未入组" "None" "$(curl -s $B/api/app/me -H "Authorization: Bearer $N_AT" | jget "['data']['family_id']")"
chk "非组长不能审批 → 403" "403" "$(code -X POST $B/api/app/family/request/decide -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"invitation_id":1,"accept":true}')"
N_RQ=$(curl -s $B/api/app/family/requests/mine -H "Authorization: Bearer $N_AT" | jget "['data']['items'][0]['id']")
chk "申请人能在「我的申请」看到自己" "True" "$([ "$N_RQ" != "None" ] && [ -n "$N_RQ" ] && echo True || echo False)"
curl -s -o /dev/null -X POST $B/api/app/family/request/decide -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$N_RQ,\"accept\":true}"
chk "审批后 N 已入组" "$FID" "$(curl -s $B/api/app/me -H "Authorization: Bearer $N_AT" | jget "['data']['family_id']")"

echo "== 5. 踢出：必须二次确认，且不能踢自己 =="
chk "非组长踢人 → 403" "403" "$(code -X POST $B/api/app/member/remove -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$N_ID,\"confirm\":true}")"
chk "组长踢人不带 confirm → 400" "400" "$(code -X POST $B/api/app/member/remove -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$N_ID}")"
chk "组长踢自己 → 400" "400" "$(code -X POST $B/api/app/member/remove -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID,\"confirm\":true}")"
chk "组长踢人（带 confirm）→ 200" "200" "$(code -X POST $B/api/app/member/remove -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$N_ID,\"confirm\":true}")"
chk "被踢者 family_id 已清空" "None" "$(curl -s $B/api/app/me -H "Authorization: Bearer $N_AT" | jget "['data']['family_id']")"
chk "被踢者被禁入该组：再申请 → 403" "403" "$(code -X POST $B/api/app/family/request -H "Authorization: Bearer $N_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}")"

echo "== 6. 退出：必须二次确认；组长不能直接退出 =="
chk "退出不带 confirm → 400" "400" "$(code -X POST $B/api/app/family/leave -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{}')"
chk "组长退出 → 409（须先转让）" "409" "$(code -X POST $B/api/app/family/leave -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"confirm":true}')"
chk "成员退出（带 confirm）→ 200" "200" "$(code -X POST $B/api/app/family/leave -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"confirm":true}')"
chk "退出后看不到组 → 403" "403" "$(code $B/api/app/family -H "Authorization: Bearer $M_AT")"

echo "== 7. 组长转让 =="
T=$(reg "t$RUN" "新组长丁" false)
T_AT=$(echo "$T" | jget "['data']['token']['access_token']")
T_ID=$(echo "$T" | jget "['data']['member']['id']")
curl -s -o /dev/null -X POST $B/api/app/family/request -H "Authorization: Bearer $T_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}"
T_RQ=$(curl -s $B/api/app/family/requests/mine -H "Authorization: Bearer $T_AT" | jget "['data']['items'][0]['id']")
curl -s -o /dev/null -X POST $B/api/app/family/request/decide -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$T_RQ,\"accept\":true}"
chk "非组长发起转让 → 403" "403" "$(code -X POST $B/api/app/family/transfer-leader -H "Authorization: Bearer $T_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID}")"
chk "组长转让给自己 → 400" "400" "$(code -X POST $B/api/app/family/transfer-leader -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID}")"
chk "组长转让给 T → 200" "200" "$(code -X POST $B/api/app/family/transfer-leader -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$T_ID}")"
chk "T 成为组长" "leader" "$(curl -s $B/api/app/me -H "Authorization: Bearer $T_AT" | jget "['data']['role']")"
chk "原组长降为成员" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $L_AT" | jget "['data']['role']")"
chk "原组长失去组长权限：设码隐藏 → 403" "403" "$(code -X POST $B/api/app/family/code/hidden -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"hidden":true}')"
chk "新组长可查家庭码 → 200" "200" "$(code $B/api/app/family/code -H "Authorization: Bearer $T_AT")"

echo "== 8. 隐藏 / 显示与改名 =="
chk "组长隐藏成员 → 200" "200" "$(code -X POST $B/api/app/member/hidden -H "Authorization: Bearer $T_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID,\"hidden\":true}")"
chk "隐藏后成员列表不含他" "False" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $T_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(m['id']==$L_ID for m in d['data']['members']))")"
curl -s -o /dev/null -X POST $B/api/app/member/hidden -H "Authorization: Bearer $T_AT" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID,\"hidden\":false}"
chk "显示后成员列表含他" "True" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $T_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(m['id']==$L_ID for m in d['data']['members']))")"
chk "组长改组名 → 200" "200" "$(code -X POST $B/api/app/family/rename -H "Authorization: Bearer $T_AT" -H 'Content-Type: application/json' -d '{"name":"改名后的家"}')"
chk "成员改组名 → 403" "403" "$(code -X POST $B/api/app/family/rename -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"name":"x"}')"
chk "本人改昵称 → 200" "200" "$(code -X POST $B/api/app/member/rename -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"nickname":"新昵称"}')"
chk "成员改名带 username → 400" "400" "$(code -X POST $B/api/app/member/rename -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"nickname":"不改","username":"hacker"}')"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
