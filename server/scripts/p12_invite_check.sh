#!/usr/bin/env bash
# P12 验收：P3.1「按用户名待确认邀请」+ username 不可改 + nickname 统一命名
# 对应 docs/features.md：5.1.6 / 5.2.3 / 5.4.9 / 15.3.43–48 / 21.2 / 22.1 / 23 节 133–143
set -u
. "$(dirname "$0")/lib_admin.sh"
# 本机 curl 默认走会话代理，会把 127.0.0.1:8080 变成 502；验收脚本必须直连。
export NO_PROXY=127.0.0.1,localhost
export no_proxy=127.0.0.1,localhost
B=http://127.0.0.1:8080
RUN=$(date +%s)
pass=0; fail=0

chk() { if [ "$2" = "$3" ]; then echo "  ✅ $1  ($3)"; pass=$((pass+1)); else echo "  ❌ $1  期望=$2 实际=$3"; fail=$((fail+1)); fi }
jget() { python -c "import sys,json;d=json.load(sys.stdin);print(eval('d'+sys.argv[1]))" "$1" 2>/dev/null; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
at()  { echo "$1" | jget "['data']['token']['access_token']"; }
mid() { echo "$1" | jget "['data']['member']['id']"; }
fid() { echo "$1" | jget "['data']['member']['family_id']"; }
# reg <username> <nickname(可空)> <create_family>
reg() {
  curl -s -X POST $B/api/register -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"Kid#2026pass\",\"nickname\":\"$2\",\"create_family\":$3,\"family_name\":\"邀请组$RUN\"}"
}
invite() { # <leader_token> <username>
  curl -s -X POST $B/api/app/member/invite -H "Authorization: Bearer $1" \
    -H 'Content-Type: application/json' -d "{\"username\":\"$2\"}"
}
decide() { # <token> <accept|reject> <invitation_id>
  curl -s -X POST "$B/api/app/invitation/$2" -H "Authorization: Bearer $1" \
    -H 'Content-Type: application/json' -d "{\"invitation_id\":$3}"
}
myinv()  { curl -s $B/api/app/invitations -H "Authorization: Bearer $1"; }
faminv() { curl -s $B/api/app/member/invitations -H "Authorization: Bearer $1"; }
devtool() { # 测试脚手架：加分 / 把邀请过期时间改到过去
  ( cd "$(dirname "$0")/.." && GOPROXY=https://goproxy.cn,direct GOSUMDB=off \
    go run ./cmd/devtool "$@" ) >/dev/null 2>&1
}
audit_count() { # <action>
  curl -s "$B/api/admin/audit/list?family_id=$FID&limit=200" -H "Authorization: Bearer $A_AT" \
    | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['action']=='$1'))" 2>/dev/null
}

echo "== 0. 造数据：两个家庭组 + 未入组用户 + 已入组成员 =="
L=$(reg "il$RUN" "组长" true);      L_AT=$(at "$L");  FID=$(fid "$L")
L2=$(reg "jl$RUN" "乙组长" true);   L2_AT=$(at "$L2"); FID2=$(fid "$L2")
X=$(reg "ix$RUN" "小新" false);     X_AT=$(at "$X");  X_ID=$(mid "$X")
Y=$(reg "iy$RUN" "小杨" false);     Y_AT=$(at "$Y")
Z=$(reg "iz$RUN" "小周" false);     Z_AT=$(at "$Z")
U=$(reg "iu$RUN" "小吴" false);     U_AT=$(at "$U")
M=$(reg "im$RUN" "已入组" false);   M_AT=$(at "$M");  M_ID=$(mid "$M")
A_AT=$(admin_login "$B")
curl -s -o /dev/null -X POST $B/api/admin/family/$FID/invite -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d "{\"username\":\"im$RUN\"}"
chk "两组建成且互不相同" "True" "$(python -c "print('$FID'!='$FID2' and '$FID'!='None')")"
chk "M 已入组" "$FID" "$(curl -s $B/api/app/me -H "Authorization: Bearer $M_AT" | jget "['data']['family_id']")"

echo "== 1. nickname 统一命名（5.1.6） =="
N=$(reg "in$RUN" "" false)
chk "注册不填昵称 → 回填 username" "in$RUN" "$(echo "$N" | jget "['data']['member']['nickname']")"
chk "注册响应没有旧 name 字段" "False" "$(echo "$N" | python -c "import sys,json;print('name' in json.load(sys.stdin)['data']['member'])")"
chk "/api/app/me 返回 nickname" "组长" "$(curl -s $B/api/app/me -H "Authorization: Bearer $L_AT" | jget "['data']['nickname']")"
chk "/api/app/me 没有 name 字段" "False" "$(curl -s $B/api/app/me -H "Authorization: Bearer $L_AT" | python -c "import sys,json;print('name' in json.load(sys.stdin)['data'])")"
chk "成员列表每项都是 nickname" "True" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(bool(d['data']['members']) and all('nickname' in m and 'name' not in m for m in d['data']['members']))")"

devtool -member "il$RUN" -points 10
curl -s -o /dev/null -X POST $B/api/app/transfer/send -H "Authorization: Bearer $L_AT" \
  -H 'Content-Type: application/json' -d "{\"to_member_id\":$M_ID,\"amount\":1,\"note\":\"快照\"}"
REC=$(curl -s "$B/api/app/records?limit=50" -H "Authorization: Bearer $L_AT")
chk "流水仍保留 member_name 快照列" "True" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(any('member_name' in r for r in d['data']['items']))")"
chk "快照是当时的昵称" "True" "$(echo "$REC" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r.get('member_name')=='已入组' for r in d['data']['items']))")"
chk "榜单行是 nickname" "True" "$(curl -s "$B/api/app/stats/ranking?type=points" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(bool(d['data']['rows']) and all('nickname' in r and 'name' not in r for r in d['data']['rows']))")"
chk "改昵称 → 200" "200" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"nickname":"改过名"}')"
chk "榜单显示新昵称" "True" "$(curl -s "$B/api/app/stats/ranking?type=points" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r['nickname']=='改过名' for r in d['data']['rows']))")"
chk "历史流水快照不被改名污染" "False" "$(curl -s "$B/api/app/records?limit=50" -H "Authorization: Bearer $L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(r.get('member_name')=='改过名' for r in d['data']['items']))")"

echo "== 2. username 永不可改（5.1.6 / 5.4.9） =="
chk "成员资料带 username → 400" "400" "$(code -X POST $B/api/app/me/profile -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"nickname":"不改","username":"hacker"}')"
chk "成员改名带 username → 400" "400" "$(code -X POST $B/api/app/member/rename -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d '{"nickname":"不改","username":"hacker"}')"
chk "username 未被改写" "im$RUN" "$(curl -s $B/api/app/me -H "Authorization: Bearer $M_AT" | jget "['data']['username']")"
chk "管理员资料带 username → 400" "400" "$(code -X POST $B/api/admin/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{"nickname":"x","username":"hacker"}')"
chk "管理员改昵称 → 200" "200" "$(code -X POST $B/api/admin/me/profile -H "Authorization: Bearer $A_AT" -H 'Content-Type: application/json' -d '{"nickname":"运营小李"}')"
chk "管理员 me 返回新昵称" "运营小李" "$(curl -s $B/api/admin/me -H "Authorization: Bearer $A_AT" | jget "['data']['nickname']")"
chk "管理员 username 未变" "admins" "$(curl -s $B/api/admin/me -H "Authorization: Bearer $A_AT" | jget "['data']['username']")"
curl -s -o /dev/null -X POST $B/api/admin/me/profile -H "Authorization: Bearer $A_AT" \
  -H 'Content-Type: application/json' -d '{"nickname":"admins"}'   # 还原，避免污染其它脚本

echo "== 3. 发起邀请（15.3.43） =="
C0=$(audit_count invite.create)
chk "用户名不存在 → 400" "400" "$(code -X POST $B/api/app/member/invite -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d '{"username":"nobody_here_x"}')"
chk "邀请自己 → 400" "400" "$(code -X POST $B/api/app/member/invite -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"username\":\"il$RUN\"}")"
chk "邀请已入组成员 → 409" "409" "$(code -X POST $B/api/app/member/invite -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"username\":\"im$RUN\"}")"
chk "普通成员发起 → 403" "403" "$(code -X POST $B/api/app/member/invite -H "Authorization: Bearer $M_AT" -H 'Content-Type: application/json' -d "{\"username\":\"ix$RUN\"}")"
chk "失败路径不写 invite.create 审计" "$C0" "$(audit_count invite.create)"
INV=$(invite "$L_AT" "ix$RUN"); INV_ID=$(echo "$INV" | jget "['data']['id']")
chk "正常发起 → ok=true" "True" "$(echo "$INV" | jget "['ok']")"
chk "返回 status=pending" "pending" "$(echo "$INV" | jget "['data']['status']")"
chk "返回体带被邀请人昵称" "小新" "$(echo "$INV" | jget "['data']['invitee_nickname']")"
chk "返回体带家庭组名" "邀请组$RUN" "$(echo "$INV" | jget "['data']['family_name']")"
chk "邀请不直接入组" "None" "$(curl -s $B/api/app/me -H "Authorization: Bearer $X_AT" | jget "['data']['family_id']")"
chk "同组重复邀请 → 409" "409" "$(code -X POST $B/api/app/member/invite -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"username\":\"ix$RUN\"}")"
chk "本组列表恰好 1 条 pending" "1" "$(faminv "$L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for i in d['data']['items'] if i['status']=='pending'))")"

echo "== 4. 未入组也能查收（15.3.46） =="
chk "未入组查收 → 200" "200" "$(code $B/api/app/invitations -H "Authorization: Bearer $X_AT")"
chk "查到发给自己的邀请" "True" "$(myinv "$X_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(i['id']==$INV_ID for i in d['data']['items']))")"
chk "带邀请人昵称" "组长" "$(myinv "$X_AT" | python -c "import sys,json;d=json.load(sys.stdin);print([i['inviter_nickname'] for i in d['data']['items'] if i['id']==$INV_ID][0])")"
chk "看不到别人的邀请" "0" "$(myinv "$Y_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for i in d['data']['items'] if i['id']==$INV_ID))")"
chk "未入组仍被挡在组内通知外（已知限制）" "403" "$(code "$B/api/app/notification/list" -H "Authorization: Bearer $X_AT")"

echo "== 5. 拒绝（15.3.48） =="
R=$(decide "$X_AT" reject "$INV_ID")
chk "拒绝 → ok=true" "True" "$(echo "$R" | jget "['ok']")"
chk "返回 status=rejected" "rejected" "$(echo "$R" | jget "['data']['status']")"
chk "decided_at 已写" "True" "$(echo "$R" | python -c "import sys,json;print(json.load(sys.stdin)['data']['decided_at'] is not None)")"
chk "重复拒绝 → 404" "404" "$(code -X POST $B/api/app/invitation/reject -H "Authorization: Bearer $X_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$INV_ID}")"
chk "再接受同一邀请 → 404" "404" "$(code -X POST $B/api/app/invitation/accept -H "Authorization: Bearer $X_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$INV_ID}")"
chk "拒绝后仍未入组" "None" "$(curl -s $B/api/app/me -H "Authorization: Bearer $X_AT" | jget "['data']['family_id']")"
chk "组长列表显示 rejected" "True" "$(faminv "$L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(i['id']==$INV_ID and i['status']=='rejected' and i['decided_at'] for i in d['data']['items']))")"

echo "== 6. 接受 + 多组互斥（15.3.47） =="
IY=$(invite "$L_AT" "iy$RUN");  IY_ID=$(echo "$IY" | jget "['data']['id']")
IY2=$(invite "$L2_AT" "iy$RUN"); IY2_ID=$(echo "$IY2" | jget "['data']['id']")
chk "不同组可各自邀同一人" "True" "$(python -c "print('$IY_ID'!='None' and '$IY2_ID'!='None' and '$IY_ID'!='$IY2_ID')")"
AY=$(decide "$Y_AT" accept "$IY_ID")
chk "接受 → ok=true" "True" "$(echo "$AY" | jget "['ok']")"
chk "返回 status=accepted" "accepted" "$(echo "$AY" | jget "['data']['status']")"
chk "重复接受 → 404" "404" "$(code -X POST $B/api/app/invitation/accept -H "Authorization: Bearer $Y_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IY_ID}")"
chk "已加入 L 组" "$FID" "$(curl -s $B/api/app/me -H "Authorization: Bearer $Y_AT" | jget "['data']['family_id']")"
chk "role=member" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $Y_AT" | jget "['data']['role']")"
chk "另一组 pending 被作废" "cancelled" "$(myinv "$Y_AT" | python -c "import sys,json;d=json.load(sys.stdin);print([i['status'] for i in d['data']['items'] if i['id']==$IY2_ID][0])")"
chk "再接受已作废邀请 → 404" "404" "$(code -X POST $B/api/app/invitation/accept -H "Authorization: Bearer $Y_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IY2_ID}")"

echo "== 7. 取消（15.3.45） =="
IZ=$(invite "$L_AT" "iz$RUN"); IZ_ID=$(echo "$IZ" | jget "['data']['id']")
chk "组长取消 → 200" "200" "$(code -X POST $B/api/app/member/invite/cancel -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IZ_ID}")"
chk "取消后该条是 cancelled" "cancelled" "$(myinv "$Z_AT" | python -c "import sys,json;d=json.load(sys.stdin);print([i['status'] for i in d['data']['items'] if i['id']==$IZ_ID][0])")"
chk "重复取消 → 404" "404" "$(code -X POST $B/api/app/member/invite/cancel -H "Authorization: Bearer $L_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IZ_ID}")"
chk "跨组取消 → 404" "404" "$(code -X POST $B/api/app/member/invite/cancel -H "Authorization: Bearer $L2_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IZ_ID}")"

echo "== 8. 过期惰性判定 + 重新邀请不死锁 =="
IE=$(invite "$L_AT" "iu$RUN"); IE_ID=$(echo "$IE" | jget "['data']['id']")
devtool -expire-invitation "$IE_ID"
chk "过期 pending 不出现在我的邀请列表" "0" "$(myinv "$U_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for i in d['data']['items'] if i['id']==$IE_ID))")"
chk "过期 pending 不出现在组长列表" "0" "$(faminv "$L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for i in d['data']['items'] if i['id']==$IE_ID))")"
chk "接受过期邀请 → 400" "400" "$(code -X POST $B/api/app/invitation/accept -H "Authorization: Bearer $U_AT" -H 'Content-Type: application/json' -d "{\"invitation_id\":$IE_ID}")"
IE2=$(invite "$L_AT" "iu$RUN"); IE2_ID=$(echo "$IE2" | jget "['data']['id']")
chk "过期后可重新邀请（唯一键已释放）" "True" "$(python -c "print('$IE2_ID'!='None' and '$IE2_ID'!='$IE_ID')")"
chk "新邀请能被查收" "True" "$(myinv "$U_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(i['id']==$IE2_ID and i['status']=='pending' for i in d['data']['items']))")"
chk "旧过期行已落为 expired" "True" "$(faminv "$L_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(i['id']==$IE_ID and i['status']=='expired' for i in d['data']['items']))")"

echo "== 9. 审计与通知（21.2 / 22.1） =="
chk "invite.accept 恰好 1 条" "1" "$(audit_count invite.accept)"
chk "invite.reject 恰好 1 条" "1" "$(audit_count invite.reject)"
chk "invite.cancel 恰好 1 条" "1" "$(audit_count invite.cancel)"
chk "invite.create ≥ 4 条" "True" "$(python -c "print(int('$(audit_count invite.create)' or 0) >= 4)")"
chk "invite.accept 的 family_id 取服务端邀请行" "True" "$(curl -s "$B/api/admin/audit/list?family_id=$FID&limit=200" -H "Authorization: Bearer $A_AT" | python -c "import sys,json;d=json.load(sys.stdin);rows=[r for r in d['data']['items'] if r['action']=='invite.accept'];print(bool(rows) and all(r['family_id']==$FID for r in rows))")"
NL=$(curl -s "$B/api/app/notification/list?limit=100" -H "Authorization: Bearer $L_AT")
chk "组长收到 invite_decided（拒绝+接受各 1）" "2" "$(echo "$NL" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='invite_decided'))")"
chk "组长不收 invite_received" "0" "$(echo "$NL" | python -c "import sys,json;d=json.load(sys.stdin);print(sum(1 for r in d['data']['items'] if r['type']=='invite_received'))")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
