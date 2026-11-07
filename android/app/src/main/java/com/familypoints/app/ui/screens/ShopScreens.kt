@file:OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)

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
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.data.api.ItemView
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.PointsText
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.nav.Routes
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

@Composable
fun ShopScreen(vm: AppViewModel, nav: NavHostController) {
    // 未加入家庭时只给占位，不去打需要家庭上下文的接口（否则满屏 403）
    val _sess by vm.session.collectAsStateWithLifecycle()
    val _noFamily = _sess is Session.NoFamily ||
        ((_sess as? Session.Ready)?.me?.familyId == null && _sess !is Session.Loading)
    if (_noFamily) {
        NoFamilyHint(nav, "商城", "商城")
        return
    }

    val items by vm.items.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    var pendingRedeem by remember { mutableStateOf<ItemView?>(null) }

    val scope = rememberCoroutineScope()
    var refreshing by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        vm.loadItems()
        vm.loadOverview()
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(
            title = "兑换商城",
            action = {
                if (me?.isLeader == true) {
                    TextButton(onClick = { nav.navigate(Routes.GOODS) }) {
                        Text("管理商品", color = Primary, fontSize = 13.sp)
                    }
                }
            },
        )

        PullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = {
                scope.launch {
                    refreshing = true
                    vm.loadItems().join()
                    vm.loadCoupons().join()
                    vm.loadOverview().join()
                    refreshing = false
                }
            },
            modifier = Modifier.fillMaxSize(),
        ) {
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(16.dp))
                        .background(Brush.linearGradient(listOf(PrimarySoft, MaterialTheme.colorScheme.surface)))
                        .padding(16.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text("我的可用积分", fontSize = 12.sp, color = Sub)
                            Text(
                                "${me?.score ?: 0}",
                                fontSize = 26.sp,
                                fontWeight = FontWeight.Bold,
                                color = Primary,
                            )
                        }
                        Text("🎁", fontSize = 32.sp)
                    }
                }
            }

            if (items.isEmpty()) {
                item { EmptyState("商城还没有商品") }
            }

            items(items, key = { it.id }) { item ->
                // 可兑换与否由后端算（affordable），前端不自己比余额
                val affordable = item.affordable
                CardBox {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .width(44.dp)
                                .height(44.dp)
                                .clip(RoundedCornerShape(13.dp))
                                .background(MaterialTheme.colorScheme.surfaceVariant),
                            contentAlignment = Alignment.Center,
                        ) { Text("🎁", fontSize = 21.sp) }
                        Spacer(Modifier.width(11.dp))
                        Column(Modifier.weight(1f)) {
                            Text(item.name, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                            Text("需要 ${item.cost} 积分", fontSize = 12.sp, color = Sub)
                        }
                        if (affordable) {
                            Button(
                                onClick = { pendingRedeem = item },
                                colors = ButtonDefaults.buttonColors(
                                    containerColor = PrimarySoft,
                                    contentColor = Primary,
                                ),
                                modifier = Modifier.height(36.dp),
                                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp),
                            ) { Text("兑换", fontSize = 13.sp) }
                        } else {
                            Text("积分不足", fontSize = 12.sp, color = Sub)
                        }
                    }
                }
            }
            item {
                CardBox(modifier = Modifier.clickable { nav.navigate(Routes.COUPONS) }) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("🎟️", fontSize = 18.sp)
                        Spacer(Modifier.width(10.dp))
                        Column(Modifier.weight(1f)) {
                            Text("我的券", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Text("已兑换的券与核销投票", fontSize = 11.5.sp, color = Sub)
                        }
                        Text("›", fontSize = 18.sp, color = Sub)
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
        } // PullToRefreshBox
    }

    pendingRedeem?.let { item ->
        AlertDialog(
            onDismissRequest = { pendingRedeem = null },
            title = { Text("确认兑换") },
            text = {
                Column {
                    Text("${item.name}", fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    Text("将扣除 ${item.cost} 积分", fontSize = 14.sp, color = Sub)
                    Spacer(Modifier.height(4.dp))
                    Text(
                        "兑换后余额 ${(me?.score ?: 0) - item.cost}",
                        fontSize = 14.sp,
                        color = Sub,
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.redeem(item.id)
                    pendingRedeem = null
                }) { Text("确认兑换", color = Primary, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = { TextButton(onClick = { pendingRedeem = null }) { Text("取消", color = Sub) } },
        )
    }
}

@Composable
fun GiftScreen(vm: AppViewModel, nav: NavHostController) {
    val members by vm.members.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    // 上限由后端 state 给出（transfer_max），前端不拿 score 自己当上限
    val transferMax by vm.transferMax.collectAsStateWithLifecycle()
    var target by remember { mutableStateOf<com.familypoints.app.data.api.MemberBrief?>(null) }

    LaunchedEffect(Unit) { vm.loadOverview() }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "赠送积分", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text("可用积分", fontSize = 12.sp, color = Sub)
                    Text(
                        "${me?.score ?: 0}",
                        fontSize = 24.sp,
                        fontWeight = FontWeight.Bold,
                        color = Primary,
                    )
                    Text(
                        "赠送是积分转移，家庭总分不变。不需要对方确认，直接到账。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }
            item {
                CardBox {
                    Text("选择成员", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(4.dp))
                    val others = members.filter { it.id != me?.id }
                    if (others.isEmpty()) EmptyState("没有其他成员")
                    others.forEachIndexed { i, m ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { target = m }
                                .padding(vertical = 11.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Avatar(m.avatar)
                            Spacer(Modifier.width(11.dp))
                            Column(Modifier.weight(1f)) {
                                Text(m.nickname, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                                Text("当前 ${m.score} 分", fontSize = 12.sp, color = Sub)
                            }
                            Text("赠送 ›", fontSize = 12.sp, color = Sub)
                        }
                        if (i != others.lastIndex) Divider(color = Line)
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    target?.let { m ->
        GiftDialog(
            targetName = m.nickname,
            max = transferMax,
            onDismiss = { target = null },
            onConfirm = { amount, note ->
                vm.sendTransfer(m.id, amount, note)
                target = null
            },
        )
    }
}

@Composable
private fun GiftDialog(
    targetName: String,
    max: Int,
    onDismiss: () -> Unit,
    onConfirm: (Int, String) -> Unit,
) {
    var amount by remember { mutableStateOf("5") }
    var note by remember { mutableStateOf("") }
    val value = amount.toIntOrNull() ?: 0

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("赠送给 $targetName") },
        text = {
            Column {
                OutlinedTextField(
                    value = amount,
                    onValueChange = { amount = it.filter { c -> c.isDigit() } },
                    label = { Text("金额") },
                    singleLine = true,
                    keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                        keyboardType = KeyboardType.Number
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = note,
                    onValueChange = { note = it },
                    label = { Text("附言（可选）") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    if (value > max) "超出可用余额（$max）" else "可用 $max 分，赠送后剩 ${max - value}",
                    fontSize = 11.5.sp,
                    color = if (value > max) MaterialTheme.colorScheme.error else Sub,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(value, note) },
                enabled = value > 0 && value <= max,
            ) { Text("确认赠送", color = Primary, fontWeight = FontWeight.SemiBold) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消", color = Sub) } },
    )
}

/** 我的券：三态 + 待我核销。 */
@Composable
fun CouponsScreen(vm: AppViewModel, nav: NavHostController) {
    val mine by vm.coupons.collectAsStateWithLifecycle()
    val pending by vm.pendingCoupons.collectAsStateWithLifecycle()
    val members by vm.members.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    LaunchedEffect(Unit) { vm.loadCoupons() }

    val unused = mine.count { it.status == "unused" }
    val waiting = mine.count { it.status == "pending" }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "我的券", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Row {
                        Column(Modifier.weight(1f)) {
                            Text("可用券", fontSize = 12.sp, color = Sub)
                            Text("$unused", fontSize = 24.sp, fontWeight = FontWeight.Bold, color = Primary)
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text("待核销", fontSize = 12.sp, color = Sub)
                            Text("$waiting", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = Blue)
                        }
                    }
                    Text(
                        "券是「已付积分待兑现」的凭证。使用后需家人投票核销。",
                        fontSize = 11.5.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }

            if (pending.isNotEmpty()) {
                item {
                    Text(
                        "待我核销（${pending.size}）",
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 14.sp,
                        modifier = Modifier.padding(start = 4.dp, top = 4.dp),
                    )
                }
                items(pending, key = { "p-${it.id}" }) { c ->
                    val owner = members.firstOrNull { it.id == c.memberId }?.nickname ?: "成员"
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Box(
                                Modifier
                                    .size(42.dp)
                                    .clip(RoundedCornerShape(13.dp))
                                    .background(MaterialTheme.colorScheme.surfaceVariant),
                                contentAlignment = Alignment.Center,
                            ) { Text("🎟️", fontSize = 20.sp) }
                            Spacer(Modifier.width(11.dp))
                            Column(Modifier.weight(1f)) {
                                Text(c.itemName, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                Text("$owner 申请核销 · 已付 ${c.cost} 分", fontSize = 12.sp, color = Sub)
                            }
                            Pill("待投票", Primary, PrimarySoft)
                        }
                        Spacer(Modifier.height(12.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            OutlinedButton(
                                onClick = { vm.voteCoupon(c.id, "reject") },
                                modifier = Modifier
                                    .weight(1f)
                                    .height(42.dp),
                            ) { Text("反对", color = Red, fontSize = 13.sp) }
                            Button(
                                onClick = { vm.voteCoupon(c.id, "approve") },
                                modifier = Modifier
                                    .weight(1f)
                                    .height(42.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Green),
                            ) { Text("同意", fontSize = 13.sp) }
                        }
                        Text(
                            "除申请人外全员过半同意才核销；投票后不可更改。",
                            fontSize = 11.sp,
                            color = Sub,
                            modifier = Modifier.padding(top = 8.dp),
                        )
                    }
                }
            }

            item {
                Text(
                    "我的券（${mine.size}）",
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                    modifier = Modifier.padding(start = 4.dp, top = 4.dp),
                )
            }

            if (mine.isEmpty()) {
                item { EmptyState("还没有券，去商城兑换一个吧") }
            }

            items(mine, key = { it.id }) { c ->
                val accent = when (c.status) {
                    "unused" -> Primary
                    "pending" -> Blue
                    else -> Green
                }
                CardBox {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(42.dp)
                                .clip(RoundedCornerShape(13.dp))
                                .background(MaterialTheme.colorScheme.surfaceVariant),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text(
                                when (c.status) {
                                    "unused" -> "🎟️"
                                    "pending" -> "⏳"
                                    else -> "✅"
                                },
                                fontSize = 20.sp,
                            )
                        }
                        Spacer(Modifier.width(11.dp))
                        Column(Modifier.weight(1f)) {
                            Text(c.itemName, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                            Text(
                                buildString {
                                    append("已付 ${c.cost} 分")
                                    if (c.rejectCount > 0) append(" · 曾被驳回 ${c.rejectCount} 次")
                                },
                                fontSize = 12.sp,
                                color = Sub,
                            )
                        }
                        when (c.status) {
                            "unused" -> Button(
                                onClick = { vm.useCoupon(c.id) },
                                modifier = Modifier.height(36.dp),
                                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp),
                                colors = ButtonDefaults.buttonColors(
                                    containerColor = PrimarySoft,
                                    contentColor = Primary,
                                ),
                            ) { Text("使用", fontSize = 13.sp) }

                            "pending" -> Pill("待家人投票", Blue, Color(0xFFEAF1FF))
                            else -> Pill("已核销", Green, Color(0xFFE8F7EE))
                        }
                    }
                    Box(
                        Modifier
                            .padding(top = 10.dp)
                            .fillMaxWidth()
                            .height(3.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(accent)
                    )
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}
