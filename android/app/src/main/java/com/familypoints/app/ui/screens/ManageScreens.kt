package com.familypoints.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.FilterChip
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.data.api.ItemView
import com.familypoints.app.data.api.ScreenRuleSaveReq
import com.familypoints.app.data.api.ScreenRuleView
import com.familypoints.app.data.api.StreakTier
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

@Composable
fun ManageMembersScreen(vm: AppViewModel, nav: NavHostController) {
    val members by vm.members.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val code by vm.inviteCode.collectAsStateWithLifecycle()
    val familyInvitations by vm.familyInvitations.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()

    val currentFamilyName by vm.familyName.collectAsStateWithLifecycle()
    val codeHidden by vm.codeHidden.collectAsStateWithLifecycle()
    val pendingRequests by vm.pendingRequests.collectAsStateWithLifecycle()
    val isLeader = me?.isLeader == true
    var familyName by remember(currentFamilyName) { mutableStateOf(currentFamilyName) }
    var inviteUsername by remember { mutableStateOf("") }
    var actionTarget by remember { mutableStateOf<com.familypoints.app.data.api.MemberBrief?>(null) }
    var confirmKick by remember { mutableStateOf<com.familypoints.app.data.api.MemberBrief?>(null) }

    LaunchedEffect(Unit) {
        vm.loadOverview()
        vm.loadInviteCode()
        vm.loadFamilyInvitations()
        vm.loadPendingRequests()
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "家庭管理", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text("家庭名称", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Text(
                        "会显示在积分榜、通知和成员页上。",
                        fontSize = 11.5.sp, color = Sub,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = familyName,
                        onValueChange = { familyName = it },
                        label = { Text("组名") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Spacer(Modifier.height(10.dp))
                    Button(
                        onClick = { vm.renameFamily(familyName) },
                        // 只挡「没改」和「空」，长度等规则交给服务端判定
                        enabled = !busy && familyName.isNotBlank() && familyName.trim() != currentFamilyName,
                        modifier = Modifier.fillMaxWidth().height(42.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Primary),
                    ) { Text("保存名称", fontWeight = FontWeight.SemiBold, fontSize = 13.sp) }
                }
            }

            item {
                CardBox {
                    Text("家庭码", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(10.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            code ?: "加载中…",
                            fontSize = 22.sp,
                            fontWeight = FontWeight.Bold,
                            color = Primary,
                            letterSpacing = 3.sp,
                            modifier = Modifier.weight(1f),
                        )
                        val clip = LocalClipboardManager.current
                        OutlinedButton(
                            onClick = {
                                code?.let {
                                    clip.setText(AnnotatedString(it))
                                    vm.showMessage("家庭码已复制")
                                }
                            },
                            modifier = Modifier.height(34.dp),
                        ) { Text("复制", fontSize = 12.5.sp, color = Sub) }
                    }
                    Text(
                        "8 位、唯一且永不可更改，建组时自动生成。别人用它在「所在家庭」里搜索本组并申请加入，需要你审批。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                    if (isLeader) {
                        Divider(color = Line, modifier = Modifier.padding(vertical = 12.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text("在搜索结果中隐藏", fontWeight = FontWeight.Medium, fontSize = 13.5.sp)
                                Text(
                                    "隐藏后别人搜不到，也无法凭码申请加入",
                                    fontSize = 11.5.sp, color = Sub,
                                )
                            }
                            Switch(checked = codeHidden, onCheckedChange = { vm.setCodeHidden(it) })
                        }
                    }
                }
            }

            // 待审批的入组申请（组长）
            if (isLeader && pendingRequests.isNotEmpty()) {
                item {
                    CardBox {
                        Text("待审批申请（${pendingRequests.size}）", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Text(
                            "对方凭家庭码申请加入，同意后立即入组。",
                            fontSize = 11.5.sp, color = Sub,
                            modifier = Modifier.padding(top = 4.dp),
                        )
                    }
                }
                items(pendingRequests, key = { "req-${it.id}" }) { inv ->
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(inv.inviteeNickname, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                                Text("申请加入 · ${inv.statusText}", fontSize = 11.5.sp, color = Sub)
                            }
                            OutlinedButton(
                                onClick = { vm.decideJoinRequest(inv.id, false) },
                                modifier = Modifier.height(34.dp),
                            ) { Text("拒绝", fontSize = 12.5.sp, color = Sub) }
                            Spacer(Modifier.width(8.dp))
                            Button(
                                onClick = { vm.decideJoinRequest(inv.id, true) },
                                modifier = Modifier.height(34.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Primary),
                            ) { Text("同意", fontSize = 12.5.sp) }
                        }
                    }
                }
            }

            item {
                CardBox {
                    Text("按用户名邀请", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(4.dp))
                    Text(
                        "对方需本人确认才会入组，不会直接拉人。用户名注册后不可修改；同组同一人同时只能有一条待确认邀请。",
                        fontSize = 11.5.sp,
                        color = Sub,
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = inviteUsername,
                        onValueChange = { inviteUsername = it },
                        label = { Text("对方的用户名") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Spacer(Modifier.height(10.dp))
                    Button(
                        onClick = {
                            vm.inviteByUsername(inviteUsername)
                            inviteUsername = ""
                        },
                        enabled = !busy && inviteUsername.isNotBlank(),
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(42.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = Primary),
                    ) { Text("发送邀请", fontWeight = FontWeight.SemiBold, fontSize = 13.sp) }
                }
            }

            item {
                CardBox {
                    Text("本组已发邀请（${familyInvitations.size}）", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(6.dp))
                    if (familyInvitations.isEmpty()) {
                        Text(
                            "暂无邀请记录",
                            fontSize = 12.5.sp,
                            color = Sub,
                            modifier = Modifier.padding(vertical = 8.dp),
                        )
                    } else {
                        familyInvitations.forEachIndexed { i, inv ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 9.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(
                                        inv.inviteeNickname.ifBlank { "成员 ${inv.inviteeId}" },
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 13.5.sp,
                                    )
                                    Text(
                                        "${inv.createdDate} 发出 · 有效期至 ${inv.expireDate}",
                                        fontSize = 11.sp,
                                        color = Sub,
                                    )
                                }
                                Pill(inv.statusText, if (inv.isPending) Primary else Sub, PrimarySoft)
                                if (inv.isPending) {
                                    Spacer(Modifier.width(6.dp))
                                    TextButton(
                                        onClick = { vm.cancelInvitation(inv.id) },
                                        modifier = Modifier.height(34.dp),
                                    ) { Text("取消", fontSize = 12.sp, color = Red) }
                                }
                            }
                            if (i != familyInvitations.lastIndex) Divider(color = Line)
                        }
                    }
                }
            }

            item {
                CardBox {
                    Text("成员（${members.size}）", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(4.dp))
                    members.forEachIndexed { i, m ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 11.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Avatar(m.avatar)
                            Spacer(Modifier.width(11.dp))
                            Column(Modifier.weight(1f)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Text(m.nickname, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                                    if (m.id == me?.id) {
                                        Spacer(Modifier.width(6.dp))
                                        Pill("我", Primary, PrimarySoft)
                                    }
                                    if (m.isLeader) {
                                        Spacer(Modifier.width(6.dp))
                                        Pill("组长", Primary, PrimarySoft)
                                    }
                                }
                                Text(
                                    "${m.score} 分${if (m.frozenScore > 0) " · 冻结 ${m.frozenScore}" else ""}",
                                    fontSize = 11.5.sp,
                                    color = Sub,
                                )
                            }
                            if (m.id != me?.id && !m.isLeader) {
                                TextButton(onClick = { actionTarget = m }) {
                                    Text("管理", fontSize = 12.5.sp, color = Sub)
                                }
                            }
                        }
                        if (i != members.lastIndex) Divider(color = Line)
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    actionTarget?.let { m ->
        AlertDialog(
            onDismissRequest = { actionTarget = null },
            title = { Text("管理 ${m.nickname}") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    OutlinedButton(
                        onClick = {
                            vm.transferLeader(m.id)
                            actionTarget = null
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("设为组长（原组长降为成员）", fontSize = 13.sp) }
                    OutlinedButton(
                        onClick = {
                            vm.setMemberHidden(m.id, true)
                            actionTarget = null
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("在榜单中隐藏", fontSize = 13.sp) }
                    OutlinedButton(
                        onClick = {
                            confirmKick = m
                            actionTarget = null
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("踢出家庭组", color = Red, fontSize = 13.sp) }
                    Text(
                        "踢出后：积分清零、禁止再加入本组，历史记录保留。",
                        fontSize = 11.5.sp,
                        color = Sub,
                    )
                }
            },
            confirmButton = {},
            dismissButton = { TextButton(onClick = { actionTarget = null }) { Text("关闭", color = Sub) } },
        )
    }

    confirmKick?.let { m ->
        AlertDialog(
            onDismissRequest = { confirmKick = null },
            title = { Text("踢出 ${m.nickname}？") },
            text = {
                Text(
                    "该成员的积分会清零，且永久无法再加入本家庭组。历史记录会保留。",
                    fontSize = 14.sp,
                    color = Sub,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.kickMember(m.id)
                    confirmKick = null
                }) { Text("确认踢出", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = { TextButton(onClick = { confirmKick = null }) { Text("取消", color = Sub) } },
        )
    }
}

@Composable
fun SleepRuleScreen(vm: AppViewModel, nav: NavHostController) {
    val members by vm.members.collectAsStateWithLifecycle()
    val screenRules by vm.screenRules.collectAsStateWithLifecycle()
    val rules by vm.rules.collectAsStateWithLifecycle()

    // 2026-11-07：适用成员默认「全部成员」（selectedId = 0 = 应用到全组）
    var selectedId by remember { mutableStateOf(0L) }
    var expanded by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { vm.loadRules() }

    val current = screenRules.firstOrNull { it.memberId == selectedId }
        ?: screenRules.firstOrNull()

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "睡觉规则", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text("适用成员", fontSize = 12.sp, color = Sub)
                    Spacer(Modifier.height(6.dp))
                    Box {
                        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
                            Text(
                                if (selectedId == 0L) "全部成员（应用到全组）"
                                else members.firstOrNull { it.id == selectedId }?.nickname
                                    ?: "全部成员（应用到全组）",
                                color = MaterialTheme.colorScheme.onSurface,
                            )
                        }
                        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                            DropdownMenuItem(
                                text = { Text("全部成员（应用到全组）") },
                                onClick = {
                                    selectedId = 0L
                                    expanded = false
                                },
                            )
                            members.forEach { m ->
                                DropdownMenuItem(
                                    text = { Text(m.nickname) },
                                    onClick = {
                                        selectedId = m.id
                                        expanded = false
                                    },
                                )
                            }
                        }
                    }
                    Text(
                        "每个家庭组各自配置；组内也能按成员分别设（如小学生 21:00、初中生 22:30）。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }

            item {
                RuleEditor(
                    rule = current,
                    onSave = { bedtime, wake, maxMin, unitMin, penaltyPoints ->
                        val targetId = selectedId
                        vm.saveScreenRule(
                                ScreenRuleSaveReq(
                                    memberId = targetId,
                                    thresholdMin = current?.thresholdMin ?: 120,
                                    bonusPoints = current?.bonusPoints ?: 2,
                                    penaltyPoints = current?.penaltyPoints ?: 3,
                                    nightStartHour = bedtime,
                                    nightEndHour = wake,
                                    nightMaxMin = maxMin,
                                    nightPenaltyUnitMin = unitMin,
                                    nightPenaltyPoints = penaltyPoints,
                                enabled = true,
                            )
                        )
                    },
                )
            }

            item {
                val rule0 = rules.firstOrNull()
                var points by remember(rule0) { mutableStateOf((rule0?.points ?: 2).toString()) }
                var tiers by remember(rule0) { mutableStateOf(rule0?.streaks.orEmpty()) }

                fun updateTier(i: Int, t: StreakTier) {
                    tiers = tiers.toMutableList().also { it[i] = t }
                }

                CardBox {
                    Text("连续奖励档位", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Text(
                        "由屏幕时间自动判定，不需要手动打卡。最多 5 档，多档独立计算、同时命中都发。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = points,
                        onValueChange = { points = it.filter { c -> c.isDigit() } },
                        label = { Text("达标奖励（分）") },
                        singleLine = true,
                        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Spacer(Modifier.height(12.dp))

                    if (tiers.isEmpty()) {
                        Text("还没有档位，点下方「添加档位」", fontSize = 12.sp, color = Sub)
                    }
                    tiers.forEachIndexed { i, t ->
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedTextField(
                                value = if (t.every == 0) "" else t.every.toString(),
                                onValueChange = { v -> updateTier(i, t.copy(every = v.filter { c -> c.isDigit() }.toIntOrNull() ?: 0)) },
                                label = { Text("连续天数") },
                                singleLine = true,
                                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                                modifier = Modifier.weight(1f),
                            )
                            OutlinedTextField(
                                value = if (t.bonus == 0) "" else t.bonus.toString(),
                                onValueChange = { v -> updateTier(i, t.copy(bonus = v.filter { c -> c.isDigit() }.toIntOrNull() ?: 0)) },
                                label = { Text("奖励分") },
                                singleLine = true,
                                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                                modifier = Modifier.weight(1f),
                            )
                        }
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 4.dp),
                        ) {
                            TextButton(onClick = {
                                updateTier(i, t.copy(mode = if (t.mode == "repeat") "once" else "repeat"))
                            }) {
                                Text(
                                    if (t.mode == "repeat") "🔁 每满就奖（点击切换）" else "1️⃣ 本轮只奖一次（点击切换）",
                                    fontSize = 12.sp, color = Primary,
                                )
                            }
                            Spacer(Modifier.weight(1f))
                            TextButton(onClick = { tiers = tiers.toMutableList().also { it.removeAt(i) } }) {
                                Text("删除", color = Red, fontSize = 12.sp)
                            }
                        }
                        if (i != tiers.lastIndex) Divider(color = Line)
                    }

                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                        TextButton(
                            onClick = { if (tiers.size < 5) tiers = tiers + StreakTier(every = 3, bonus = 5, mode = "repeat") },
                            enabled = tiers.size < 5,
                        ) { Text("＋ 添加档位", color = Primary) }
                        Spacer(Modifier.weight(1f))
                        Button(
                            onClick = {
                                val pt = points.toIntOrNull() ?: 0
                                val clean = tiers.filter { it.every >= 1 && it.bonus != 0 }
                                vm.saveRule(rule0?.id, rule0?.name ?: "按时睡觉", pt, clean)
                            },
                            colors = ButtonDefaults.buttonColors(containerColor = Primary),
                            modifier = Modifier.height(40.dp),
                        ) { Text("保存规则") }
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}

@Composable
private fun RuleEditor(
    rule: ScreenRuleView?,
    onSave: (Int, Int, Int, Int, Int) -> Unit,
) {
    var bedtime by remember(rule) { mutableStateOf((rule?.nightStartHour ?: 0).toString()) }
    var wake by remember(rule) { mutableStateOf((rule?.nightEndHour ?: 7).toString()) }
    var maxMin by remember(rule) { mutableStateOf((rule?.nightMaxMin ?: 0).toString()) }
    // 扣分口径：单位 30/60 分钟，每单位扣 N 分（后端按「超出部分向上取整」计算）
    var unitMin by remember(rule) { mutableStateOf(if (rule?.nightPenaltyUnitMin == 60) 60 else 30) }
    var penaltyPoints by remember(rule) {
        mutableStateOf((rule?.nightPenaltyPoints ?: 0).toString())
    }

    CardBox {
        Text("判定条件", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
        Spacer(Modifier.height(10.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = bedtime,
                onValueChange = { bedtime = it.filter { c -> c.isDigit() } },
                label = { Text("睡觉时间点（0=24点）") },
                singleLine = true,
                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.weight(1f),
            )
            OutlinedTextField(
                value = wake,
                onValueChange = { wake = it.filter { c -> c.isDigit() } },
                label = { Text("起床时间点") },
                singleLine = true,
                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.weight(1f),
            )
        }
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(
            value = maxMin,
            onValueChange = { maxMin = it.filter { c -> c.isDigit() } },
            label = { Text("该时段允许使用（分钟，0=完全不许用）") },
            singleLine = true,
            keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.fillMaxWidth(),
        )
        Text(
            "例：睡觉点 0、起床 7、允许 0 分钟 = 「24 点前放下手机」，用一分钟就算未达标。",
            fontSize = 11.5.sp,
            color = Sub,
            modifier = Modifier.padding(top = 8.dp),
        )

        Spacer(Modifier.height(14.dp))
        Text("未达标的扣分", fontWeight = FontWeight.Medium, fontSize = 13.sp)
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf(30 to "每半小时", 60 to "每小时").forEach { (u, label) ->
                FilterChip(
                    selected = unitMin == u,
                    onClick = { unitMin = u },
                    label = { Text(label) },
                )
            }
        }
        Spacer(Modifier.height(8.dp))
        OutlinedTextField(
            value = penaltyPoints,
            onValueChange = { penaltyPoints = it.filter { c -> c.isDigit() } },
            label = { Text("每单位扣几分（0 = 不扣分）") },
            singleLine = true,
            keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.fillMaxWidth(),
        )
        Text(
            "超出允许分钟数的部分，按单位**向上取整**后每单位扣这么多分。" +
                "例：允许 0 分钟、用了 100 分钟、单位半小时、每单位 2 分 → 4 个单位 → 扣 8 分。",
            fontSize = 11.5.sp,
            color = Sub,
            modifier = Modifier.padding(top = 8.dp),
        )
        Spacer(Modifier.height(14.dp))
        Button(
            onClick = {
                // 原样提交，**不在前端钳制**：输 25 点就该被后端拒绝并显示原因，
                // 静默改成 23 只会让家长以为保存成功了。
                onSave(
                    bedtime.toIntOrNull() ?: -1,
                    wake.toIntOrNull() ?: -1,
                    maxMin.toIntOrNull() ?: -1,
                    unitMin,
                    penaltyPoints.toIntOrNull() ?: -1,
                )
            },
            modifier = Modifier
                .fillMaxWidth()
                .height(46.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Primary),
        ) { Text("保存规则") }
    }
}

@Composable
fun GoodsScreen(vm: AppViewModel, nav: NavHostController) {
    val items by vm.items.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<ItemView?>(null) }
    var creating by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { vm.loadItems() }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "商品管理", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (items.isEmpty()) item { EmptyState("还没有商品") }

            items(items, key = { it.id }) { item ->
                CardBox {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(44.dp)
                                .clip(RoundedCornerShape(13.dp))
                                .background(MaterialTheme.colorScheme.surfaceVariant),
                            contentAlignment = Alignment.Center,
                        ) { Text("🎁", fontSize = 21.sp) }
                        Spacer(Modifier.width(11.dp))
                        Column(Modifier.weight(1f)) {
                            Text(item.name, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                            Text("${item.cost} 积分", fontSize = 12.sp, color = Sub)
                        }
                        TextButton(onClick = { editing = item }) {
                            Text("编辑", fontSize = 12.5.sp, color = Primary)
                        }
                        TextButton(onClick = { vm.deleteItem(item.id) }) {
                            Text("删", fontSize = 12.5.sp, color = Red)
                        }
                    }
                }
            }

            item {
                Button(
                    onClick = { creating = true },
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(46.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = Primary),
                ) { Text("+ 新增商品") }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (creating || editing != null) {
        ItemDialog(
            item = editing,
            onDismiss = {
                creating = false
                editing = null
            },
            onConfirm = { name, cost ->
                vm.saveItem(editing?.id, name, cost)
                creating = false
                editing = null
            },
        )
    }
}

@Composable
private fun ItemDialog(
    item: ItemView?,
    onDismiss: () -> Unit,
    onConfirm: (String, Int) -> Unit,
) {
    var name by remember { mutableStateOf(item?.name ?: "") }
    var cost by remember { mutableStateOf((item?.cost ?: 10).toString()) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (item == null) "新增商品" else "编辑商品") },
        text = {
            Column {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("名称") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = cost,
                    onValueChange = { cost = it.filter { c -> c.isDigit() } },
                    label = { Text("所需积分") },
                    singleLine = true,
                    keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(name, cost.toIntOrNull() ?: 0) },
                enabled = name.isNotBlank() && (cost.toIntOrNull() ?: 0) > 0,
            ) { Text("保存", color = Primary, fontWeight = FontWeight.SemiBold) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消", color = Sub) } },
    )
}
