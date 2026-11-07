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
import com.familypoints.app.data.local.ReportStatusStore
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
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** 上报时间戳 → 「MM-dd HH:mm」。 */
private fun fmtClock(ts: Long): String =
    SimpleDateFormat("MM-dd HH:mm", Locale.CHINA).format(Date(ts))

@Composable
fun ProfileScreen(vm: AppViewModel, nav: NavHostController) {
    val session by vm.session.collectAsStateWithLifecycle()
    // 未入组时 session 是 NoFamily，但 me 仍然要读出来——否则「我的」页会显示空昵称/空用户名
    val me = when (val s = session) {
        is Session.Ready -> s.me
        is Session.NoFamily -> s.me
        else -> null
    }
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
                        // 左：身份（我是谁）—— 昵称可改，用户名是登录凭证、永不可改。
                        Column(Modifier.weight(1f)) {
                            Text(
                                me?.nickname.orEmpty().ifBlank { "我" },
                                fontWeight = FontWeight.Bold,
                                fontSize = 17.sp,
                            )
                            Text(
                                "用户名 ${me?.username.orEmpty()}",
                                fontSize = 12.sp,
                                color = Sub,
                            )
                        }
                        // 家庭与职位不再放这里——下面的「所在家庭」与家庭页各有更合适的位置，
                        // 一张卡上重复三处信息只会更乱（2026-11-06 反馈）。
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

            // 所在家庭：右侧显示组名，点进去看成员 / 家庭码 / 退出
            item {
                CardBox(modifier = Modifier.clickable { nav.navigate(Routes.FAMILY) }) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(34.dp)
                                .clip(RoundedCornerShape(11.dp))
                                .background(MaterialTheme.colorScheme.surfaceVariant),
                            contentAlignment = Alignment.Center,
                        ) { Text("🏡", fontSize = 16.sp) }
                        Spacer(Modifier.width(11.dp))
                        Column(Modifier.weight(1f)) {
                            Text("所在家庭", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                            Text(
                                if (me?.familyId != null) "查看成员与家庭码" else "还没有加入家庭",
                                fontSize = 11.5.sp, color = Sub,
                            )
                        }
                        Text(
                            familyName.ifBlank { "未加入" },
                            fontSize = 13.5.sp,
                            color = if (me?.familyId != null) Sub else Primary,
                        )
                        Spacer(Modifier.width(6.dp))
                        Text("›", fontSize = 18.sp, color = Sub)
                    }
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
                    MenuItem("📋", "积分流水", "每一条加分与扣分") { nav.navigate(Routes.RECORDS) }
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
    // 未加入家庭：屏幕时间没有可上报/可查看的组，给占位而不是空页面
    val _sess by vm.session.collectAsStateWithLifecycle()
    val _noFamily = _sess is Session.NoFamily ||
        ((_sess as? Session.Ready)?.me?.familyId == null && _sess !is Session.Loading)
    if (_noFamily) {
        NoFamilyHint(nav, "屏幕时间", "屏幕时间")
        return
    }

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

    // 上报状态的真实情况：Worker 每次成败都会写入 ReportStatusStore。
    // 这里不能无条件显示「正常」——ROM 杀掉定时任务 / 接口连不上时，用户必须能看出来。
    val reportStatus by remember { ReportStatusStore(context) }.status
        .collectAsStateWithLifecycle(initialValue = null)
    val reportPill: Triple<String, Color, Color>
    val reportDesc: String
    val nowMs = remember(detail) { System.currentTimeMillis() }
    when {
        reportStatus == null -> {
            reportPill = Triple("…", Sub, Line)
            reportDesc = "读取中"
        }
        reportStatus!!.lastSuccessAt == null -> {
            reportPill = Triple("尚未上报", Color(0xFFD97706), Color(0xFFFFF6E8))
            reportDesc = "已授权但还没有成功上报过" +
                (reportStatus!!.lastError?.let { "；最近一次失败：${it.take(80)}" } ?: "")
        }
        nowMs - reportStatus!!.lastSuccessAt!! > 36L * 60 * 60 * 1000 -> {
            reportPill = Triple("上报异常", Color(0xFFD97706), Color(0xFFFFF6E8))
            reportDesc = "上次成功：${fmtClock(reportStatus!!.lastSuccessAt!!)}" +
                (reportStatus!!.lastError?.takeIf { reportStatus!!.lastFailAt ?: 0 > reportStatus!!.lastSuccessAt!! }
                    ?.let { "；最近失败：${it.take(80)}" } ?: "")
        }
        else -> {
            reportPill = Triple("正常", Green, GreenSoft)
            reportDesc = "上次成功：${fmtClock(reportStatus!!.lastSuccessAt!!)}（每天 21:00 自动上报昨天）"
        }
    }

    // 国产 ROM（HyperOS / EMUI / ColorOS 等）默认禁止自启动：
    // 进程一旦被杀或重启手机，WorkManager 的定时任务不会再跑——只能由用户手动放行。
    val needsKeepAliveHint = remember {
        listOf("xiaomi", "redmi", "huawei", "honor", "oppo", "vivo", "oneplus", "realme", "meizu")
            .any { android.os.Build.MANUFACTURER.lowercase().contains(it) }
    }
    val keepAliveHint = when {
        android.os.Build.MANUFACTURER.lowercase().contains("xiaomi") ||
            android.os.Build.MANUFACTURER.lowercase().contains("redmi") ->
            "小米手机请到「设置 → 应用设置 → 应用管理 → ${context.packageName}」打开「自启动」，" +
                "并把省电策略设为「无限制」；否则 App 被杀后定时上报不会再运行，" +
                "重新打开一次 App 可以立即补算。"
        else ->
            "请到系统设置里允许本应用「自启动」/「后台运行」，并把电池策略设为不限制；" +
                "否则 App 被杀后定时上报不会再运行，重新打开一次 App 可以立即补算。"
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
                                Text("使用情况访问权限", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text(
                                    if (hasPermission) "已授权，可自动统计" else "未授权，无法统计",
                                    fontSize = 12.sp, color = Sub,
                                )
                            }
                            Pill(
                                if (hasPermission) "已授权" else "未授权",
                                if (hasPermission) Green else Color(0xFFD97706),
                                if (hasPermission) GreenSoft else Color(0xFFFFF6E8),
                            )
                        }
                        Divider(color = Line, modifier = Modifier.padding(vertical = 12.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text("上报状态", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text(reportDesc, fontSize = 12.sp, color = Sub, lineHeight = 18.sp)
                            }
                            Pill(reportPill.first, reportPill.second, reportPill.third)
                        }
                    }
                }

                // 国产 ROM 保活引导（12.2.8）：进程被杀成 stopped 后 WorkManager 不再运行，
                // 这是「已授权却收不到数据」在小米/HyperOS 等系统上的真实原因。
                if (needsKeepAliveHint) {
                    item {
                        CardBox {
                            Text("手机管家可能拦截了定时上报", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                            Text(
                                keepAliveHint,
                                fontSize = 12.5.sp, color = Sub, lineHeight = 19.sp,
                                modifier = Modifier.padding(top = 8.dp),
                            )
                            Button(
                                onClick = {
                                    runCatching {
                                        context.startActivity(
                                            Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS)
                                                .setData(android.net.Uri.fromParts("package", context.packageName, null))
                                                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                                        )
                                    }
                                },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp)
                                    .padding(top = 14.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Primary),
                            ) { Text("打开应用信息") }
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
