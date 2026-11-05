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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
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
    // 近 7 天明细（最旧在前、今天在最后），一次 queryEvents 分桶得到
    var days by remember { mutableStateOf<List<DayUsage>>(emptyList()) }
    var loading by remember { mutableStateOf(false) }
    var pickedHour by remember { mutableStateOf<Int?>(null) }
    val deviceToken by vm.deviceToken.collectAsStateWithLifecycle()
    var tokenInput by remember { mutableStateOf("") }
    val scope = rememberCoroutineScope()

    // 夜间时段必须用**本机登录成员自己的规则**：写死 0–7 会和实际计分口径不一致
    // （多成员家庭里小学生和初中生的睡觉时间点本来就可以不同，见 docs 25 条）。
    val session by vm.session.collectAsStateWithLifecycle()
    val rules by vm.screenRules.collectAsStateWithLifecycle()
    val myId = (session as? Session.Ready)?.me?.id ?: 0L
    val myRule = rules.firstOrNull { it.memberId == myId } ?: rules.firstOrNull()
    val nightStart = myRule?.nightStartHour ?: 0
    val nightEnd = myRule?.nightEndHour ?: 7
    val nightMax = myRule?.nightMaxMin ?: 0

    // ⚠️ 判定权在服务端：是否达标、扣多少分都由后端算（规则全部由后端判断）。
    // 前端只展示结算行里的 night_ok；null = 还没上报/结算，显示「待结算」而不是猜一个。
    val reports by vm.screenReports.collectAsStateWithLifecycle()
    // 只认「昨天」那条结算行：更早的旧报告不能拿来显示今晚的达标状态
    val yesterdayDate = reader.dateString(1)
    val latestReport = reports.firstOrNull { it.reportDate.take(10) == yesterdayDate }
    val nightVerdict: Boolean? = latestReport?.nightOk

    // 「已同步」不能靠猜：只有「有权限 + 配了设备令牌」才可能上报成功。
    // 之前这里用 loadOverview() 是否成功来推断，结果是**永远显示已同步**，
    // 而没有设备令牌时 Worker 会直接 return，根本不会上报。
    val synced = hasPermission && !deviceToken.isNullOrBlank()

    // 读事件流要遍历全部 UsageEvents，放主线程会卡首帧，统一丢 IO。
    suspend fun reload() {
        loading = true
        val list = withContext(Dispatchers.IO) {
            runCatching { reader.readBuckets(BUCKET_DAYS) }.getOrDefault(emptyList())
        }
        days = list
        val today = list.lastOrNull()
        todayMinutes = today?.totalMinutes ?: 0
        nightMinutes = today?.nightMinutes(nightStart, nightEnd) ?: 0
        loading = false
        vm.loadScreenReports() // 判定以服务端为准
    }

    // 规则加载完成后要重算一次夜间分钟数（规则是异步拿到的）
    LaunchedEffect(nightStart, nightEnd) {
        hasPermission = reader.hasPermission()
        if (hasPermission) reload()
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
                                        when (nightVerdict) {
                                            true -> "达标"
                                            false -> "未达标"
                                            null -> "待结算"
                                        },
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 15.sp,
                                        color = when (nightVerdict) {
                                            true -> Green
                                            false -> Red
                                            null -> Sub
                                        },
                                    )
                                    Text(
                                        "昨晚 · 上限 ${nightMax} 分钟",
                                        fontSize = 11.5.sp,
                                        color = Sub,
                                    )
                                }
                            }
                        }
                    }
                }

                item {
                    HourlyCard(
                        today = days.lastOrNull(),
                        pickedHour = pickedHour,
                        nightEnd = nightEnd,
                        onPick = { pickedHour = it },
                    )
                }

                item {
                    DailyCard(days = days, loading = loading)
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
                        onClick = {
                            scope.launch {
                                reload()
                                vm.showMessage("已刷新本地统计")
                            }
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

// ---------- 屏幕时间明细：每时 / 每日 ----------
//
// 数据全部来自设备本地（UsageStatsManager），服务端只存日总量、不存小时桶 ——
// 上传 24 个桶 × 每人 × 每天没有实际用途，还让上报体从几十字节涨到几百字节。

private const val BUCKET_DAYS = 7

private fun fmtMinutes(min: Int): String =
    if (min >= 60) "${min / 60}h${min % 60}m" else "${min}m"

private fun dayLabel(date: String): String {
    val today = java.time.LocalDate.now()
    val d = runCatching { java.time.LocalDate.parse(date) }.getOrNull() ?: return date
    return when (d) {
        today -> "今天"
        today.minusDays(1) -> "昨天"
        else -> "%02d-%02d".format(d.monthValue, d.dayOfMonth)
    }
}

/** 今日 24 小时分布。点柱子可看该小时的具体分钟数。 */
@Composable
private fun HourlyCard(
    today: DayUsage?,
    pickedHour: Int?,
    nightEnd: Int,
    onPick: (Int?) -> Unit,
) {
    val hourly = today?.hourlyMinutes ?: IntArray(24)
    val maxMin = (hourly.maxOrNull() ?: 0).coerceAtLeast(1)
    val peakHour = hourly.indices.maxByOrNull { hourly[it] } ?: 0

    CardBox {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("今日每小时", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.weight(1f))
            Text(
                pickedHour?.let {
                    val end = (it + 1) % 24
                    "%02d:00–%02d:00 · %d 分钟".format(it, end, hourly.getOrElse(it) { 0 })
                } ?: "点柱子看该小时",
                fontSize = 11.5.sp,
                color = if (pickedHour == null) Sub else Primary,
            )
        }
        Spacer(Modifier.height(12.dp))

        Row(
            Modifier
                .fillMaxWidth()
                .height(88.dp),
            verticalAlignment = Alignment.Bottom,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            for (h in 0..23) {
                val v = hourly[h]
                val frac = (v.toFloat() / maxMin).coerceIn(0f, 1f)
                val color = when {
                    pickedHour == h -> Ink
                    v == 0 -> Line
                    h < nightEnd -> Blue.copy(alpha = 0.45f) // 夜间时段，与「昨晚睡觉」呼应
                    else -> Primary
                }
                Box(
                    Modifier
                        .weight(1f)
                        .fillMaxHeight(if (v == 0) 0.04f else frac.coerceAtLeast(0.08f))
                        .clip(RoundedCornerShape(topStart = 3.dp, topEnd = 3.dp))
                        .background(color)
                        .clickable { onPick(if (pickedHour == h) null else h) }
                )
            }
        }

        Spacer(Modifier.height(6.dp))
        Row(Modifier.fillMaxWidth()) {
            listOf("0", "6", "12", "18", "23").forEachIndexed { i, label ->
                Text(
                    label,
                    fontSize = 10.sp,
                    color = Sub,
                    modifier = Modifier.weight(1f),
                    textAlign = when (i) {
                        0 -> TextAlign.Start
                        4 -> TextAlign.End
                        else -> TextAlign.Center
                    },
                )
            }
        }

        if (hourly.any { it > 0 }) {
            Text(
                "最多使用：${peakHour} 点 · ${hourly[peakHour]} 分钟",
                fontSize = 11.5.sp,
                color = Sub,
                modifier = Modifier.padding(top = 8.dp),
            )
        } else {
            Text(
                "今天还没有使用记录",
                fontSize = 11.5.sp,
                color = Sub,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
    }
}

/** 近 7 天每日总时长（今天在最上面）。 */
@Composable
private fun DailyCard(days: List<DayUsage>, loading: Boolean) {
    CardBox {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("近 $BUCKET_DAYS 天每日", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.weight(1f))
            Text("按天累计", fontSize = 11.5.sp, color = Sub)
        }
        Spacer(Modifier.height(10.dp))

        if (days.isEmpty()) {
            Text(
                if (loading) "正在统计…" else "暂无数据，需要先授予「使用情况访问」权限",
                fontSize = 12.sp,
                color = Sub,
                lineHeight = 18.sp,
            )
            return@CardBox
        }

        val maxDay = (days.maxOfOrNull { it.totalMinutes } ?: 0).coerceAtLeast(1)
        days.reversed().forEach { d ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .padding(vertical = 5.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    dayLabel(d.date),
                    fontSize = 12.sp,
                    color = if (dayLabel(d.date) == "今天") Ink else Sub,
                    fontWeight = if (dayLabel(d.date) == "今天") FontWeight.SemiBold else FontWeight.Normal,
                    modifier = Modifier.width(48.dp),
                )
                Box(
                    Modifier
                        .weight(1f)
                        .height(8.dp)
                        .clip(RoundedCornerShape(4.dp))
                        .background(Line),
                ) {
                    if (d.totalMinutes > 0) {
                        Box(
                            Modifier
                                .fillMaxWidth(d.totalMinutes.toFloat() / maxDay)
                                .fillMaxHeight()
                                .clip(RoundedCornerShape(4.dp))
                                .background(Primary),
                        )
                    }
                }
                Text(
                    fmtMinutes(d.totalMinutes),
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.End,
                    modifier = Modifier.width(58.dp),
                )
            }
        }

        Text(
            "数值为设备本地统计，与服务端上报口径一致（排除桌面与系统界面）",
            fontSize = 11.sp,
            color = Sub,
            lineHeight = 17.sp,
            modifier = Modifier.padding(top = 6.dp),
        )
    }
}
