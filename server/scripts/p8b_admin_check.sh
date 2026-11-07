#!/usr/bin/env bash
# P8b 后台管理接口 + CSV 导出 验收
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

A_AT=$(admin_login "$B")
AH="Authorization: Bearer $A_AT"

echo "== 1. 造数据 =="
L=$(reg "l9$RUN" "组长" true); L_AT=$(echo "$L" | jget "['data']['token']['access_token']"); L_ID=$(echo "$L" | jget "['data']['member']['id']")
FID=$(curl -s $B/api/app/family -H "Authorization: Bearer $L_AT" | jget "['data']['id']")
# 一个未入组的用户，供后台邀请
OUT=$(reg "o9$RUN" "待邀请" false); OUT_AT=$(echo "$OUT" | jget "['data']['token']['access_token']"); OUT_ID=$(echo "$OUT" | jget "['data']['member']['id']")
echo "  组=$FID 组长=$L_ID 待邀请=$OUT_ID"

echo "== 2. 全站视图 =="
chk "家庭组列表 200" "200" "$(code $B/api/admin/families -H "$AH")"
chk "列表含本组" "True" "$(curl -s $B/api/admin/families -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(any(f['id']==$FID for f in d['data']['families']))")"
chk "列表带成员数" "True" "$(curl -s $B/api/admin/families -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(all('member_count' in f for f in d['data']['families']))")"
chk "全站统计 200" "200" "$(code $B/api/admin/state -H "$AH")"
chk "统计有 family_count" "True" "$(curl -s $B/api/admin/state -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(d['data']['family_count']>0)")"

echo "== 3. 后台成员管理 =="
chk "邀请未入组用户 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"o9$RUN\"}")"
chk "被邀请者已入组" "$FID" "$(curl -s $B/api/app/me -H "Authorization: Bearer $OUT_AT" | jget "['data']['family_id']")"
chk "重复邀请 → 409" "409" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"o9$RUN\"}")"
chk "邀请不存在的用户 → 400" "400" "$(code -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d '{"username":"nobody_here_x"}')"
chk "踢出组长 → 409（须先换组长）" "409" "$(code -X POST $B/api/admin/family/$FID/member/kick -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID}")"
chk "后台设新组长 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/member/leader -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$OUT_ID}")"
chk "新组长角色生效" "leader" "$(curl -s $B/api/app/me -H "Authorization: Bearer $OUT_AT" | jget "['data']['role']")"
chk "原组长降为 member" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $L_AT" | jget "['data']['role']")"
chk "隐藏成员 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/member/hidden -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID,\"hidden\":true}")"
chk "隐藏后成员列表不含他" "False" "$(curl -s $B/api/app/family/members -H "Authorization: Bearer $OUT_AT" | python -c "import sys,json;d=json.load(sys.stdin);print(any(m['id']==$L_ID for m in d['data']['members']))")"
chk "后台改组名 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/rename -H "$AH" -H 'Content-Type: application/json' -d '{"name":"后台改的名"}')"
chk "组名已改" "后台改的名" "$(curl -s $B/api/app/family -H "Authorization: Bearer $OUT_AT" | jget "['data']['name']")"

echo "== 3b. 封禁名单与加入尝试（15.2.38/39，只读）=="
# 踢掉原组长 L（现在他是普通成员）制造一条 ban；两条 app 侧路径各造一条邀请/申请
curl -s -o /dev/null -X POST $B/api/admin/family/$FID/member/kick -H "$AH" -H 'Content-Type: application/json' -d "{\"member_id\":$L_ID}"
E9=$(reg "e9$RUN" "受邀者" false)
curl -s -o /dev/null -X POST $B/api/app/member/invite -H "Authorization: Bearer $OUT_AT" -H 'Content-Type: application/json' -d "{\"username\":\"e9$RUN\"}"
R9=$(reg "r9$RUN" "申请人" false); R9_AT=$(echo "$R9" | jget "['data']['token']['access_token']")
CODE9=$(curl -s $B/api/app/family/code -H "Authorization: Bearer $OUT_AT" | jget "['data']['code']")
curl -s -o /dev/null -X POST $B/api/app/family/request -H "Authorization: Bearer $R9_AT" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE9\"}"
bans_json() { curl -s $B/api/admin/family/$FID/bans -H "$AH"; }
att_json() { curl -s "$B/api/admin/family/$FID/invitations?limit=200" -H "$AH"; }
chk "封禁名单 200" "200" "$(code $B/api/admin/family/$FID/bans -H "$AH")"
chk "名单含被踢者（reason=kicked）" "True" "$(bans_json | python -c "import sys,json;print(any(b['member_id']==$L_ID and b['reason']=='kicked' for b in json.load(sys.stdin)['data']['bans']))")"
chk "名单带 username" "l9$RUN" "$(bans_json | python -c "import sys,json;d=json.load(sys.stdin);print([b['username'] for b in d['data']['bans'] if b['member_id']==$L_ID][0])")"
chk "加入尝试 200" "200" "$(code "$B/api/admin/family/$FID/invitations" -H "$AH")"
chk "含组长的邀请（direction=invite）" "True" "$(att_json | python -c "import sys,json;print(any(a['direction']=='invite' and a['invitee_username']=='e9$RUN' for a in json.load(sys.stdin)['data']['attempts']))")"
chk "含凭码申请（direction=request, pending）" "True" "$(att_json | python -c "import sys,json;print(any(a['direction']=='request' and a['status']=='pending' and a['invitee_username']=='r9$RUN' for a in json.load(sys.stdin)['data']['attempts']))")"
chk "列表带处理时间字段" "True" "$(att_json | python -c "import sys,json;print(all('decided_at' in a for a in json.load(sys.stdin)['data']['attempts']))")"

echo "== 4. 后台配置（与组长同权）=="
chk "后台存规则 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/rule/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"按时睡觉","points":4,"streaks":[{"every":2,"bonus":3,"mode":"repeat"}]}')"
chk "规则对成员生效" "4" "$(curl -s $B/api/app/rule -H "Authorization: Bearer $OUT_AT" | jget "['data']['rules'][0]['points']")"
chk "后台非法档位 → 400" "400" "$(code -X POST $B/api/admin/family/$FID/rule/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"x","points":1,"streaks":[{"every":0,"bonus":1}]}')"
chk "后台存商品 → 200" "200" "$(code -X POST $B/api/admin/family/$FID/item/save -H "$AH" -H 'Content-Type: application/json' -d '{"name":"看电影","cost":20}')"
chk "商品对成员可见" "看电影" "$(curl -s $B/api/app/item/list -H "Authorization: Bearer $OUT_AT" | jget "['data']['items'][0]['name']")"

echo "== 5. CSV 导出 =="
HDR=$(curl -s -D - -o /tmp/exp.csv "$B/api/admin/export/records?family_id=$FID" -H "$AH" | tr -d '\r')
chk "导出 200 + text/csv" "True" "$(echo "$HDR" | grep -qi 'text/csv' && echo True || echo False)"
chk "带附件头" "True" "$(echo "$HDR" | grep -qi 'attachment' && echo True || echo False)"
chk "CSV 有表头" "时间,成员,标题,分值,来源" "$(head -1 /tmp/exp.csv | sed 's/^\xEF\xBB\xBF//')"
chk "缺 family_id → 400" "400" "$(code "$B/api/admin/export/records" -H "$AH")"

echo "== 6. 管理员账号 =="
chk "账号列表 200" "200" "$(code $B/api/admin/accounts -H "$AH")"
chk "列表含 admins" "True" "$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print(any(a['username']=='admins' for a in d['data']['admins']))")"
chk "列表不含 pass_hash" "False" "$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;print('pass_hash' in json.dumps(json.load(sys.stdin)))")"
chk "新增管理员 → 200" "200" "$(code -X POST $B/api/admin/accounts -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}")"
NEW_ID=$(curl -s $B/api/admin/accounts -H "$AH" | python -c "import sys,json;d=json.load(sys.stdin);print([a['id'] for a in d['data']['admins'] if a['username']=='ops$RUN'][0])")
chk "新管理员首登被要求改密" "True" "$(curl -s -X POST $B/api/admin/login -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}" | jget "['data']['token']['must_change_password']")"
ME_ID=$(curl -s $B/api/admin/me -H "$AH" | jget "['data']['id']")
chk "停用自己 → 400" "400" "$(code -X POST $B/api/admin/accounts/disable -H "$AH" -H 'Content-Type: application/json' -d "{\"admin_id\":$ME_ID,\"disabled\":true}")"
chk "停用新账号 → 200" "200" "$(code -X POST $B/api/admin/accounts/disable -H "$AH" -H 'Content-Type: application/json' -d "{\"admin_id\":$NEW_ID,\"disabled\":true}")"
chk "停用后登录 → 403" "403" "$(code -X POST $B/api/admin/login -H 'Content-Type: application/json' -d "{\"username\":\"ops$RUN\",\"password\":\"Ops#2026pass\"}")"

echo "== 7. 越权 =="
chk "成员 token 打后台 → 401" "401" "$(code $B/api/admin/families -H "Authorization: Bearer $OUT_AT")"
chk "成员 token 导出 → 401" "401" "$(code "$B/api/admin/export/records?family_id=$FID" -H "Authorization: Bearer $OUT_AT")"

echo "== 8. 解散家庭组（5.5.2，2026-11-07）=="
# 用一个全新的组来解散，避免污染前面断言的组
DL=$(reg "dl$RUN" "解散组长" true); DL_AT=$(echo "$DL" | jget "['data']['token']['access_token']"); DL_ID=$(echo "$DL" | jget "['data']['member']['id']")
DFID=$(curl -s $B/api/app/family -H "Authorization: Bearer $DL_AT" | jget "['data']['id']")
DM=$(reg "dm$RUN" "解散成员" false); DM_AT=$(echo "$DM" | jget "['data']['token']['access_token']"); DM_ID=$(echo "$DM" | jget "['data']['member']['id']")
curl -s -o /dev/null -X POST $B/api/admin/family/$DFID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\"}"
curl -s -o /dev/null -X POST $B/api/app/device/unbind/request -H "Authorization: Bearer $DM_AT" -H 'Content-Type: application/json' -d '{"device_key":"dlx"}'  # 未绑定设备 → 404，仅确认校验存在
BEFORE_SCORE=$(curl -s $B/api/app/me -H "Authorization: Bearer $DM_AT" | jget "['data']['score']")
chk "确认组名不对 → 400" "400" "$(code -X POST $B/api/admin/family/$DFID/dissolve -H "$AH" -H 'Content-Type: application/json' -d '{"confirm_name":"错的组名"}')"
chk "确认组名一致 → 200" "200" "$(code -X POST $B/api/admin/family/$DFID/dissolve -H "$AH" -H 'Content-Type: application/json' -d "{\"confirm_name\":\"组$RUN\"}")"
chk "解散后再解散 → 404" "404" "$(code -X POST $B/api/admin/family/$DFID/dissolve -H "$AH" -H 'Content-Type: application/json' -d "{\"confirm_name\":\"组$RUN\"}")"
chk "组长失去组长身份" "member" "$(curl -s $B/api/app/me -H "Authorization: Bearer $DL_AT" | jget "['data']['role']")"
chk "成员 family_id 清空" "None" "$(curl -s $B/api/app/me -H "Authorization: Bearer $DM_AT" | jget "['data']['family_id']")"
chk "家庭组列表不再含它" "False" "$(curl -s $B/api/admin/families -H "$AH" | python -c "import sys,json;print(any(f['id']==$DFID for f in json.load(sys.stdin)['data']['families']))")"
INV=$(curl -s -X POST $B/api/admin/family/$FID/invite -H "$AH" -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\"}")
chk "被解散的成员可以加入其他组" "200" "$(echo "$INV" | python -c 'import sys,json;print(200 if json.load(sys.stdin).get("ok") else 0)')"

echo "== 9. 用户反馈（15.2.41/42 + 15.3.60，2026-11-07）=="
FB_AT=$(echo "$DM" | jget "['data']['token']['access_token']")  # dm 已在 FID 组；反馈无需入组
curl -s -o /dev/null -X POST $B/api/app/feedback -H "Authorization: Bearer $FB_AT" -H 'Content-Type: application/json' -d '{"content":"   "}'
chk "空内容 → 400" "400" "$(code -X POST $B/api/app/feedback -H "Authorization: Bearer $FB_AT" -H 'Content-Type: application/json' -d '{"content":"   "}')"
LONG=$(python -c 'print("字"*501)')
chk "超 500 字 → 400" "400" "$(code -X POST $B/api/app/feedback -H "Authorization: Bearer $FB_AT" -H 'Content-Type: application/json' -d "{\"content\":\"$LONG\"}")"
chk "正常提交 → 200" "200" "$(code -X POST $B/api/app/feedback -H "Authorization: Bearer $FB_AT" -H 'Content-Type: application/json' -d "{\"content\":\"冒烟反馈$RUN\"}")"
FB_ID=$(curl -s "$B/api/admin/feedbacks?handled=0&limit=50" -H "$AH" | python -c "import sys,json;print([f['id'] for f in json.load(sys.stdin)['data']['items'] if f['content']=='冒烟反馈$RUN'][0])")
chk "后台列表含它（带用户名快照）" "True" "$(curl -s "$B/api/admin/feedbacks?handled=0" -H "$AH" | python -c "import sys,json;print(any(f['id']==$FB_ID and f['username']=='dm$RUN' for f in json.load(sys.stdin)['data']['items']))")"
chk "标记已处理 → 200" "200" "$(code -X POST $B/api/admin/feedback/$FB_ID/handle -H "$AH")"
chk "重复标记 → 404" "404" "$(code -X POST $B/api/admin/feedback/$FB_ID/handle -H "$AH")"
chk "已处理列表含它" "True" "$(curl -s "$B/api/admin/feedbacks?handled=1" -H "$AH" | python -c "import sys,json;print(any(f['id']==$FB_ID for f in json.load(sys.stdin)['data']['items']))")"

echo "== 10. 用户账号管理（15.2.44–46，2026-11-07）=="
chk "全局列表 200" "200" "$(code "$B/api/admin/members?limit=200" -H "$AH")"
chk "列表含 dm 且带家庭名" "True" "$(curl -s "$B/api/admin/members?keyword=dm$RUN" -H "$AH" | python -c "import sys,json;print(all('family_name' in f for f in json.load(sys.stdin)['data']['items']))")"
chk "停用 → 200" "200" "$(code -X POST $B/api/admin/member/$DM_ID/disable -H "$AH" -H 'Content-Type: application/json' -d '{"disabled":true}')"
chk "停用后登录 → 403" "403" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\",\"password\":\"Kid#2026pass\"}")"
chk "停用后原 token 调接口 → 401" "401" "$(code $B/api/app/me -H "Authorization: Bearer $DM_AT")"
chk "启用 → 200" "200" "$(code -X POST $B/api/admin/member/$DM_ID/disable -H "$AH" -H 'Content-Type: application/json' -d '{"disabled":false}')"
chk "启用后能登录" "200" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\",\"password\":\"Kid#2026pass\"}")"
chk "重置密码 → 200" "200" "$(code -X POST $B/api/admin/member/$DM_ID/reset-password -H "$AH" -H 'Content-Type: application/json' -d '{"password":"NewPass#123"}')"
chk "旧密码登录 → 401" "401" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\",\"password\":\"Kid#2026pass\"}")"
chk "新密码登录 → 200" "200" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"dm$RUN\",\"password\":\"NewPass#123\"}")"
chk "重置密码过短 → 400" "400" "$(code -X POST $B/api/admin/member/$DM_ID/reset-password -H "$AH" -H 'Content-Type: application/json' -d '{"password":"123"}')"

echo "== 11. 手机号/邮箱注册与开关（15.1.1/1a，2026-11-07）=="
# 默认开关关闭：直接注册 → 400
chk "开关关闭时手机号注册 → 400" "400" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"phone\",\"phone\":\"138$([ $(echo $RUN | tail -c 9) ] && echo $RUN | tail -c 8)001\",\"password\":\"Kid#2026pass\",\"nickname\":\"手机用户\"}")"
# 打开开关
curl -s -o /dev/null -X POST $B/api/admin/site-config -H "$AH" -H 'Content-Type: application/json' -d '{"icp":"","reg_phone":true,"reg_email":true}'
OPT=$(curl -s $B/api/register/options)
chk "options 公开可读" "200" "$(code $B/api/register/options)"
chk "options.phone=true" "True" "$(echo "$OPT" | python -c "import sys,json;print(json.load(sys.stdin)['data']['phone'])")"
PH="138$(printf '%08d' $((RUN % 100000000)))"
chk "手机号注册 → 200" "200" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"phone\",\"phone\":\"$PH\",\"password\":\"Kid#2026pass\",\"nickname\":\"手机用户\"}")"
chk "重复手机号 → 409" "409" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"phone\",\"phone\":\"$PH\",\"password\":\"Kid#2026pass\",\"nickname\":\"重复\"}")"
chk "手机号登录 → 200" "200" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"$PH\",\"password\":\"Kid#2026pass\"}")"
EM="t$RUN@example.com"
chk "邮箱注册 → 200" "200" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"email\",\"email\":\"$EM\",\"password\":\"Kid#2026pass\",\"nickname\":\"邮箱用户\"}")"
chk "邮箱登录 → 200" "200" "$(code -X POST $B/api/app/login -H 'Content-Type: application/json' -d "{\"username\":\"$EM\",\"password\":\"Kid#2026pass\"}")"
chk "坏手机号 → 400" "400" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"phone\",\"phone\":\"12345\",\"password\":\"Kid#2026pass\"}")"
# 关闭开关再注册 → 400
curl -s -o /dev/null -X POST $B/api/admin/site-config -H "$AH" -H 'Content-Type: application/json' -d '{"icp":"","reg_phone":false,"reg_email":false}'
chk "关闭后手机号注册 → 400" "400" "$(code -X POST $B/api/register -H 'Content-Type: application/json' -d "{\"mode\":\"phone\",\"phone\":\"139$(echo $RUN | tail -c 8)99\",\"password\":\"Kid#2026pass\"}")"

echo
echo "通过 $pass 项，失败 $fail 项"
[ "$fail" -eq 0 ]
