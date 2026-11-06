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
import androidx.compose.foundation.layout.fillMaxHeight
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
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.Lifecycle
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
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
import com.familypoints.app.screen.DayUsage
import com.familypoints.app.screen.UsageStatsReader
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.nav.Routes
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Ink
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.util.DeviceInfo
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

@Composable
fun ProfileScreen(vm: AppViewModel, nav: NavHostController) {
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val familyName by vm.familyName.collectAsStateWithLifecycle()
    val unread by vm.unread.collectAsStateWithLifecycle()
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
                CardBox(modifier = Modifier.clickable { nav.navigate(Routes.PROFILE_EDIT) }) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Avatar(me?.avatar, size = 52)
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(
                                me?.nickname.orEmpty().ifBlank { "我" },
                                fontWeight = FontWeight.Bold,
                                fontSize = 17.sp,
                            )
                            // 用户名（username）与昵称是两回事：前者登录用、永不可改，
                            // 后者可改。这里都显示，避免用户分不清「我叫什么」和「我用什么登录」。
                            Text(
                                "用户名 ${me?.username.orEmpty()}",
                                fontSize = 12.sp,
                                color = Sub,
                            )
                            Text(
                                "${if (me?.isLeader == true) "组长" else "成员"} · $familyName",
                                fontSize = 11.5.sp,
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
                        Spacer(Modifier.width(6.dp))
                        Text("›", fontSize = 18.sp, color = Sub)
                    }
                    Text(
                        "点头像可修改名称与头像",
                        fontSize = 11.sp, color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }

            item {
                CardBox {
                    Text("积分管理", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    // 只读一行：当前可用积分。与「赠送」同组，因为这两件事是同一件事的两面。
                    StatRow(
                        icon = "💰",
                        title = "可用积分",
                        subtitle = "当前可以花掉的积分",
                        value = "${me?.score ?: 0}",
                    )
                    Divider(color = Line)
                    MenuItem("🎁", "赠送积分", "把积分转给其他成员") { nav.navigate(Routes.GIFT) }
                }
            }

            item {
                CardBox {
                    MenuItem(
                        "🔔", "通知",
                        if (unread > 0) "有 $unread 条未读" else "投票提醒与结果",
                        badge = unread,
                    ) { nav.navigate(Routes.NOTIFY) }
                    Divider(color = Line)
                    MenuItem("📋", "积分记录", "全部流水") { nav.navigate(Routes.RECORDS) }
                }
            }

            if (me?.isLeader == true) {
                item {
                    CardBox {
                        Text("组长工具", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                        Spacer(Modifier.height(6.dp))
                        MenuItem("👥", "家庭管理", "改组名、邀请、踢出、设组长") { nav.navigate(Routes.MANAGE_MEMBERS) }
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
private fun MenuItem(icon: String, title: String, subtitle: String, badge: Int = 0, onClick: () -> Unit) {
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
        if (badge > 0) {
            Box(
                Modifier
                    .clip(RoundedCornerShape(99.dp))
                    .background(Red)
                    .padding(horizontal = 7.dp, vertical = 2.dp),
            ) {
                Text(
                    if (badge > 99) "99+" else "$badge",
                    color = androidx.compose.ui.graphics.Color.White,
                    fontSize = 10.5.sp, fontWeight = FontWeight.SemiBold,
                )
            }
            Spacer(Modifier.width(6.dp))
        }
        Text("›", fontSize = 18.sp, color = Sub)
    }
}

/**
 * 只读信息行：与 [MenuItem] 同一套排版，但没有箭头、不可点，右侧显示数值。
 * 用在「可用积分」这类只展示不跳转的条目上，避免做成按钮让人以为能点。
 */
@Composable
private fun StatRow(icon: String, title: String, subtitle: String, value: String) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
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
        Text(value, fontWeight = FontWeight.Bold, fontSize = 18.sp)
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
fun ScreenTimeScreen(vm: AppViewModel, nav: NavHostController, asTab: Boolean = false) {
    val context = LocalContext.current
    val reader = remember { UsageStatsReader(context) }
    var hasPermission by remember { mutableStateOf(reader.hasPermission()) }
    var loading by remember { mutableStateOf(false) }
    // null = 全部设备相加；非空 = 只看这一台
    var pickedDevice by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    val detail by vm.screenDetail.collectAsStateWithLifecycle()
    val familyScreen by vm.familyScreen.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val rules by vm.screenRules.collectAsStateWithLifecycle()
    val myId = (session as? Session.Ready)?.me?.id ?: 0L
    val myRule = rules.firstOrNull { it.memberId == myId } ?: rules.firstOrNull()
    val nightStart = myRule?.nightStartHour ?: 0
    val nightEnd = myRule?.nightEndHour ?: 7
    val nightMax = myRule?.nightMaxMin ?: 0

    // 刷新 = 先本机采一次并上传「今天」的快照，**再**读服务端明细。
    // 顺序不能反：快照还没落库就去读，看到的是上一次的数据。
    suspend fun refresh() {
        loading = true
        if (reader.hasPermission()) {
            val buckets = withContext(Dispatchers.IO) {
                runCatching { reader.readBuckets(1) }.getOrDefault(emptyList())
            }
            val today = buckets.lastOrNull()
            if (today != null) {
                vm.uploadTodaySnapshot(
                    deviceKey = DeviceInfo.key(context),
                    deviceModel = DeviceInfo.model(),
                    hours = today.hourlyMinutes,
                    nightMin = today.nightMinutes(nightStart, nightEnd),
                    date = today.date,
                )
            }
        }
        vm.loadScreenDetail(0) // 0 = 自己
        vm.loadFamilyScreen()
        loading = false
    }

    LaunchedEffect(nightStart, nightEnd) {
        hasPermission = reader.hasPermission()
        if (hasPermission) refresh()
    }

    // 从系统设置授权后返回：Activity 不会重建，必须监听 ON_RESUME 重新检测，
    // 否则页面一直停在「需要权限」，用户以为没生效（2026-11-05 反馈）。
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val obs = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                val ok = reader.hasPermission()
                hasPermission = ok
                if (ok) scope.launch { refresh() }
            }
        }
        lifecycleOwner.lifecycle.addObserver(obs)
        onDispose { lifecycleOwner.lifecycle.removeObserver(obs) }
    }

    // 换设备后旧选择可能已不存在（比如设备被移除），回落到「全部」
    LaunchedEffect(detail) {
        val sel = pickedDevice ?: return@LaunchedEffect
        if (detail?.devices?.none { it.deviceKey == sel } != false) pickedDevice = null
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "屏幕时间", onBack = if (asTab) null else ({ nav.popBackStack() }))

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
                val d = detail
                if (d == null) {
                    item {
                        CardBox {
                            Text(
                                if (loading) "正在读取…" else "还没有屏幕时间数据",
                                fontSize = 13.sp, color = Sub,
                            )
                        }
                    }
                } else {
                    item { UsageTotalCard(d, pickedDevice) }
                    if (d.devices.size > 1) {
                        item {
                            DeviceChips(d.devices, pickedDevice) { pickedDevice = it }
                        }
                    }
                    item { HourlyBars(d.hourlyOf(pickedDevice), nightStart, nightEnd) }
                    item { DailyBars(d.days, d.daysOf(pickedDevice)) }
                    if (pickedDevice == null && d.devices.size > 1) {
                        item { DeviceBreakdown(d.devices) }
                    }
                }

                item {
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text("上报状态", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text(
                                    "已授权，按计划自动上报（设备：${DeviceInfo.model()}）",
                                    fontSize = 12.sp,
                                    color = Sub,
                                )
                            }
                            Pill("正常", Green, GreenSoft)
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

                // 自己已经在最上面展示过了，这里只列**其他成员**。
                // 只有一个人的家庭整段不显示——否则会看到一张只有自己名字、
                // 点进去还和自己一模一样内容的卡片，纯噪音。
                val others = familyScreen?.members.orEmpty().filter { it.memberId != myId }
                if (others.isNotEmpty()) {
                    item {
                        CardBox {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text("其他成员", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                                Spacer(Modifier.weight(1f))
                                Text(
                                    familyScreen?.date ?: "",
                                    fontSize = 11.5.sp, color = Sub,
                                )
                            }
                            Text(
                                "点任意成员可查看他的每小时与近 7 天明细。总时长 = 该成员全部设备相加。",
                                fontSize = 11.5.sp, color = Sub,
                                modifier = Modifier.padding(top = 4.dp),
                            )
                        }
                    }
                    items(others, key = { "fm-${it.memberId}" }) { m ->
                        CardBox(modifier = Modifier.clickable {
                            nav.navigate("screenMember/${m.memberId}")
                        }) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text(m.nickname, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                    Text(
                                        if (m.devices.isEmpty()) "今天还没有上报"
                                        else "${fmtMinutes(m.totalMin)} · ${m.devices.size} 台设备",
                                        fontSize = 12.sp, color = Sub,
                                    )
                                }
                                NightVerdictPill(m.nightOk, m.nightOk != null)
                                Spacer(Modifier.width(6.dp))
                                Text("›", fontSize = 18.sp, color = Sub)
                            }
                        }
                    }
                }

                item {
                    CardBox {
                        Text("上报规则", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "每天 21:00 自动上报「昨天」的完整数据；打开 App 时会额外上传今天的实时快照。\n" +
                                "重复上报会覆盖同一天的结果，不会重复计分。\n" +
                                "夜间时段（${nightStart}:00–${nightEnd}:00）的数据记在前一天账上。\n" +
                                "是否达标、扣多少分**全部由服务端按规则计算**，本页只展示结果。",
                            fontSize = 12.sp,
                            color = Sub,
                            lineHeight = 19.sp,
                        )
                    }
                }

                item {
                    Button(
                        onClick = { scope.launch { refresh(); vm.showMessage("已刷新") } },
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(46.dp),
                        colors = ButtonDefaults.buttonColors(containerColor = PrimarySoft, contentColor = Primary),
                    ) { Text(if (loading) "刷新中…" else "刷新数据") }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}


/** 底部导航的「屏幕」tab：与详情页同一套内容，只是不带返回键。 */
@Composable
fun ScreenTabScreen(vm: AppViewModel, nav: NavHostController) {
    ScreenTimeScreen(vm, nav, asTab = true)
}
