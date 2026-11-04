package com.familypoints.app.ui.screens

import android.content.Intent
import android.provider.Settings
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.screen.UsageStatsReader
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.nav.Routes
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

@Composable
fun ProfileScreen(vm: AppViewModel, nav: NavHostController) {
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val familyName by vm.familyName.collectAsStateWithLifecycle()
    var confirmLogout by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { vm.loadOverview() }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "我的")

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Avatar(me?.avatar, size = 52)
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(
                                me?.name.orEmpty().ifBlank { "我" },
                                fontWeight = FontWeight.Bold,
                                fontSize = 17.sp,
                            )
                            Text(
                                "${if (me?.isLeader == true) "组长" else "成员"} · $familyName",
                                fontSize = 12.sp,
                                color = Sub,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text(
                                "${me?.score ?: 0}",
                                fontWeight = FontWeight.Bold,
                                fontSize = 19.sp,
                            )
                            Text("可用积分", fontSize = 11.sp, color = Sub)
                        }
                    }
                }
            }

            item {
                CardBox {
                    MenuItem("🎁", "赠送积分", "把积分转给其他成员") { nav.navigate(Routes.GIFT) }
                    Divider(color = Line)
                    MenuItem("📱", "屏幕时间", "今日使用与上报状态") { nav.navigate(Routes.SCREEN_TIME) }
                    Divider(color = Line)
                    MenuItem("📋", "积分记录", "全部流水") { nav.navigate(Routes.RECORDS) }
                }
            }

            if (me?.isLeader == true) {
                item {
                    CardBox {
                        Text("组长工具", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                        Spacer(Modifier.height(6.dp))
                        MenuItem("👥", "成员管理", "邀请、踢出、设组长") { nav.navigate(Routes.MANAGE_MEMBERS) }
                        Divider(color = Line)
                        MenuItem("🌙", "睡觉规则", "时间点与连续奖励") { nav.navigate(Routes.SLEEP_RULE) }
                        Divider(color = Line)
                        MenuItem("🏪", "商品管理", "增删改商城商品") { nav.navigate(Routes.GOODS) }
                    }
                }
            }

            item {
                CardBox {
                    MenuItem("🚪", "退出登录", "") { confirmLogout = true }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (confirmLogout) {
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirmLogout = false },
            title = { Text("退出登录？") },
            text = { Text("退出后需要重新输入用户名密码。", fontSize = 14.sp, color = Sub) },
            confirmButton = {
                TextButton(onClick = {
                    confirmLogout = false
                    vm.logout()
                }) { Text("退出", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = { TextButton(onClick = { confirmLogout = false }) { Text("取消", color = Sub) } },
        )
    }
}

@Composable
private fun MenuItem(icon: String, title: String, subtitle: String, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onClick() }
            .padding(vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .size(34.dp)
                .clip(RoundedCornerShape(11.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant),
            contentAlignment = Alignment.Center,
        ) { Text(icon, fontSize = 16.sp) }
        Spacer(Modifier.width(11.dp))
        Column(Modifier.weight(1f)) {
            Text(title, fontWeight = FontWeight.Medium, fontSize = 14.sp)
            if (subtitle.isNotBlank()) Text(subtitle, fontSize = 11.5.sp, color = Sub)
        }
        Text("›", fontSize = 18.sp, color = Sub)
    }
}

@Composable
fun NotificationsScreen(vm: AppViewModel, nav: NavHostController) {
    val list by vm.notifications.collectAsStateWithLifecycle()
    val unread by vm.unread.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { vm.loadNotifications() }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(
            title = "通知",
            action = {
                if (unread > 0) {
                    TextButton(onClick = { vm.readAllNotifications() }) {
                        Text("全部已读", color = Primary, fontSize = 13.sp)
                    }
                }
            },
        )
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (list.isEmpty()) {
                item { EmptyState("暂无通知") }
            }
            items(list, key = { it.id }) { n ->
                CardBox(
                    modifier = Modifier.clickable {
                        if (n.unread) vm.readNotification(n.id)
                    }
                ) {
                    Row(verticalAlignment = Alignment.Top) {
                        Box(
                            Modifier
                                .size(36.dp)
                                .clip(RoundedCornerShape(11.dp))
                                .background(MaterialTheme.colorScheme.surfaceVariant),
                            contentAlignment = Alignment.Center,
                        ) { Text(if (n.unread) "🔔" else "📄", fontSize = 16.sp) }
                        Spacer(Modifier.width(11.dp))
                        Column(Modifier.weight(1f)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    n.title,
                                    fontWeight = if (n.unread) FontWeight.Bold else FontWeight.Medium,
                                    fontSize = 14.sp,
                                    modifier = Modifier.weight(1f),
                                )
                                if (n.unread) {
                                    Box(
                                        Modifier
                                            .size(7.dp)
                                            .clip(CircleShape)
                                            .background(Primary)
                                    )
                                }
                            }
                            if (!n.body.isNullOrBlank()) {
                                Text(n.body, fontSize = 12.5.sp, color = Sub, modifier = Modifier.padding(top = 2.dp))
                            }
                            Text(fmtTime(n.createdAt), fontSize = 11.sp, color = Sub, modifier = Modifier.padding(top = 4.dp))
                        }
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}

@Composable
fun ScreenTimeScreen(vm: AppViewModel, nav: NavHostController) {
    val context = LocalContext.current
    val reader = remember { UsageStatsReader(context) }
    var hasPermission by remember { mutableStateOf(reader.hasPermission()) }
    var todayMinutes by remember { mutableStateOf(0) }
    var nightMinutes by remember { mutableStateOf(0) }
    val deviceToken by vm.deviceToken.collectAsStateWithLifecycle()
    var tokenInput by remember { mutableStateOf("") }

    // 「已同步」不能靠猜：只有「有权限 + 配了设备令牌」才可能上报成功。
    // 之前这里用 loadOverview() 是否成功来推断，结果是**永远显示已同步**，
    // 而没有设备令牌时 Worker 会直接 return，根本不会上报。
    val synced = hasPermission && !deviceToken.isNullOrBlank()

    LaunchedEffect(Unit) {
        hasPermission = reader.hasPermission()
        if (hasPermission) {
            val from = reader.startOfDay(0)
            val to = System.currentTimeMillis()
            val r = reader.read(from, to, 0, 7, 0)
            todayMinutes = r.totalMinutes
            nightMinutes = r.nightMinutes
        }
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "屏幕时间", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (!hasPermission) {
                item {
                    CardBox {
                        Text("需要「使用情况访问」权限", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                        Text(
                            "Android 要求由你本人在系统设置里手动开启，应用无法自行申请。" +
                                "开启后会自动统计每天的使用时长，并在每晚定时上报。",
                            fontSize = 12.5.sp,
                            color = Sub,
                            lineHeight = 19.sp,
                            modifier = Modifier.padding(top = 8.dp),
                        )
                        Spacer(Modifier.height(14.dp))
                        Button(
                            onClick = {
                                runCatching {
                                    context.startActivity(
                                        Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS)
                                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                                    )
                                }
                            },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(46.dp),
                            colors = ButtonDefaults.buttonColors(containerColor = Primary),
                        ) { Text("去开启权限") }
                    }
                }
            } else {
                item {
                    CardBox {
                        Column(
                            Modifier.fillMaxWidth(),
                            horizontalAlignment = Alignment.CenterHorizontally,
                        ) {
                            Box(
                                Modifier
                                    .size(132.dp)
                                    .clip(CircleShape)
                                    .background(
                                        androidx.compose.ui.graphics.Brush.sweepGradient(
                                            listOf(Primary, PrimarySoft, Primary)
                                        )
                                    ),
                                contentAlignment = Alignment.Center,
                            ) {
                                Box(
                                    Modifier
                                        .size(110.dp)
                                        .clip(CircleShape)
                                        .background(MaterialTheme.colorScheme.surface),
                                    contentAlignment = Alignment.Center,
                                ) {
                                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                        Text(
                                            "${todayMinutes / 60}h${todayMinutes % 60}m",
                                            fontSize = 25.sp,
                                            fontWeight = FontWeight.Bold,
                                        )
                                        Text("今日使用", fontSize = 11.5.sp, color = Sub)
                                    }
                                }
                            }
                            Spacer(Modifier.height(14.dp))
                            Row(
                                Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceEvenly,
                            ) {
                                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                    Text("$nightMinutes 分钟", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                    Text("夜间使用", fontSize = 11.5.sp, color = Sub)
                                }
                                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                    Text(
                                        if (nightMinutes == 0) "达标" else "未达标",
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 15.sp,
                                        color = if (nightMinutes == 0) Green else Red,
                                    )
                                    Text("昨晚睡觉", fontSize = 11.5.sp, color = Sub)
                                }
                            }
                        }
                    }
                }

                item {
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text("上报状态", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text(
                                    when {
                                        !hasPermission -> "缺少使用情况访问权限"
                                        deviceToken.isNullOrBlank() -> "未配置设备令牌，无法上报"
                                        else -> "已配置设备令牌，按计划自动上报"
                                    },
                                    fontSize = 12.sp,
                                    color = Sub,
                                )
                            }
                            Pill(
                                if (synced) "正常" else "待配置",
                                if (synced) Green else Red,
                                if (synced) GreenSoft else MaterialTheme.colorScheme.surfaceVariant,
                            )
                        }
                        Divider(color = Line, modifier = Modifier.padding(vertical = 12.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text("使用情况访问权限", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text("已授权，可自动统计", fontSize = 12.sp, color = Sub)
                            }
                            Pill("已授权", Green, GreenSoft)
                        }
                    }
                }

                if (deviceToken.isNullOrBlank()) {
                    item {
                        CardBox {
                            Text("配置设备令牌", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Text(
                                "屏幕时间只接受设备令牌上报（成员登录态打不进这个接口）。" +
                                    "请让管理员在 Web 后台「设备管理」里为这个成员生成一个令牌，" +
                                    "粘贴到下面——明文只显示一次，存好再关。",
                                fontSize = 12.sp,
                                color = Sub,
                                lineHeight = 18.sp,
                                modifier = Modifier.padding(top = 6.dp, bottom = 10.dp),
                            )
                            OutlinedTextField(
                                value = tokenInput,
                                onValueChange = { tokenInput = it },
                                label = { Text("设备令牌") },
                                singleLine = true,
                                modifier = Modifier.fillMaxWidth(),
                            )
                            Spacer(Modifier.height(10.dp))
                            Button(
                                onClick = { vm.saveDeviceToken(tokenInput) },
                                modifier = Modifier.fillMaxWidth().height(44.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Primary, contentColor = androidx.compose.ui.graphics.Color.White),
                            ) { Text("保存并立即上报") }
                        }
                    }
                }

                item {
                    CardBox {
                        Text("上报规则", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "每天 21:00 自动上报「昨天」的完整数据；打开 App 时也会补算一次。\n" +
                                "重复上报会覆盖同一天的结果，不会重复计分。\n" +
                                "夜间时段（0:00–7:00）的数据记在前一天账上。",
                            fontSize = 12.sp,
                            color = Sub,
                            lineHeight = 19.sp,
                        )
                    }
                }

                item {
                    Button(
                        onClick = {
                            val from = reader.startOfDay(1)
                            val to = reader.startOfDay(0)
                            val r = reader.read(from, to, 0, 7, 0)
                            todayMinutes = r.totalMinutes
                            nightMinutes = r.nightMinutes
                            vm.showMessage("已刷新本地统计")
                        },
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(46.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = PrimarySoft, contentColor = Primary),
                    ) { Text("重新统计") }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}
