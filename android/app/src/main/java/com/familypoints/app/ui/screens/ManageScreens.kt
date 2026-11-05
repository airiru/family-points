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
import androidx.compose.ui.text.font.FontWeight
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

    var actionTarget by remember { mutableStateOf<com.familypoints.app.data.api.MemberBrief?>(null) }
    var confirmKick by remember { mutableStateOf<com.familypoints.app.data.api.MemberBrief?>(null) }

    LaunchedEffect(Unit) {
        vm.loadOverview()
        vm.loadInviteCode()
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "成员管理", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text("邀请码", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Text("有效期 7 天", fontSize = 11.5.sp, color = Sub)
                        }
                        OutlinedButton(onClick = { vm.resetInviteCode() }, modifier = Modifier.height(36.dp)) {
                            Text("重置", fontSize = 12.5.sp, color = Sub)
                        }
                    }
                    Spacer(Modifier.height(10.dp))
                    Text(
                        code ?: "加载中…",
                        fontSize = 22.sp,
                        fontWeight = FontWeight.Bold,
                        color = Primary,
                        letterSpacing = 3.sp,
                    )
                    Text(
                        "对方注册后输入此码即可加入。重置后旧码立即失效。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
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
                                    Text(m.name, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
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
            title = { Text("管理 ${m.name}") },
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
            title = { Text("踢出 ${m.name}？") },
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

    var selectedId by remember { mutableStateOf<Long?>(null) }
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
                                members.firstOrNull { it.id == (selectedId ?: current?.memberId) }?.name
                                    ?: "选择成员",
                                color = MaterialTheme.colorScheme.onSurface,
                            )
                        }
                        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                            members.forEach { m ->
                                DropdownMenuItem(
                                    text = { Text(m.name) },
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
                        val targetId = selectedId ?: current?.memberId ?: members.firstOrNull()?.id
                        if (targetId != null) {
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
                        }
                    },
                )
            }

            item {
                CardBox {
                    Text("连续奖励档位", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Text(
                        "由屏幕时间自动判定，不需要手动打卡。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    Spacer(Modifier.height(10.dp))
                    val tiers = rules.firstOrNull()?.streaks.orEmpty()
                    if (tiers.isEmpty()) {
                        Text("尚未配置档位", fontSize = 12.sp, color = Sub)
                    } else {
                        tiers.forEachIndexed { i, t ->
                            Row(
                                Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 9.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(
                                        "连续 ${t.every} 天 +${t.bonus} 分",
                                        fontWeight = FontWeight.Medium,
                                        fontSize = 14.sp,
                                    )
                                    Text(
                                        if (t.mode == "repeat") "每满就奖" else "本轮只奖一次",
                                        fontSize = 11.5.sp,
                                        color = Sub,
                                    )
                                }
                            }
                            if (i != tiers.lastIndex) Divider(color = Line)
                        }
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
