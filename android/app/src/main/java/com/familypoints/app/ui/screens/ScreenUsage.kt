package com.familypoints.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredWidth
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.familypoints.app.data.api.DayPoint
import com.familypoints.app.data.api.MemberDetail
import com.familypoints.app.data.api.MemberDeviceDetail
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.RedSoft
import com.familypoints.app.ui.theme.Sub

/**
 * 屏幕时间的展示组件（2026-11-06）。
 *
 * 这些组件**只画服务端返回的数字**：不补零、不重算、不做阈值判断。
 * 小时桶与每日合计都由 `GET /api/app/screen/member/:id` 算好，
 * 前端的职责只有「选哪台设备」和「画成什么形状」。
 */

/** 把分钟数写成「2 小时 40 分」/「18 分钟」。 */
internal fun fmtMinutes(min: Int): String =
    if (min >= 60) "${min / 60} 小时 ${min % 60} 分" else "$min 分钟"

/** 空选择（null）= 全部设备相加。 */
internal fun MemberDetail.hourlyOf(deviceKey: String?): List<Int> {
    if (deviceKey == null) return hourly
    return devices.firstOrNull { it.deviceKey == deviceKey }?.hourly ?: List(24) { 0 }
}

internal fun MemberDetail.daysOf(deviceKey: String?): List<Int> {
    if (deviceKey == null) return days.map { it.totalMin }
    val d = devices.firstOrNull { it.deviceKey == deviceKey }
    return d?.days ?: List(days.size) { 0 }
}

internal fun MemberDetail.totalOf(deviceKey: String?): Int {
    if (deviceKey == null) return totalMin
    return devices.firstOrNull { it.deviceKey == deviceKey }?.totalMin ?: 0
}

internal fun MemberDetail.nightOf(deviceKey: String?): Int {
    if (deviceKey == null) return nightMin ?: 0
    return devices.firstOrNull { it.deviceKey == deviceKey }?.nightMin ?: 0
}

/** 判定标签：未结算时显示「待结算」，绝不猜。 */
@Composable
internal fun NightVerdictPill(nightOk: Boolean?, settled: Boolean) {
    val (text, fg, bg) = when {
        nightOk == true -> Triple("按时睡", Green, GreenSoft)
        nightOk == false -> Triple("超时", Red, RedSoft)
        !settled -> Triple("待结算", Sub, MaterialTheme.colorScheme.surfaceVariant)
        else -> Triple("待结算", Sub, MaterialTheme.colorScheme.surfaceVariant)
    }
    Pill(text, fg, bg)
}

/**
 * 顶部总览：所选口径的总时长 + 夜间分钟 + 判定。
 *
 * 「全部设备」时标题写「全部设备合计」，「单台设备」时写型号——
 * 避免用户把「这台手机」误读成「我今天总共用了多久」。
 */
@Composable
internal fun UsageTotalCard(detail: MemberDetail, selected: String?) {
    val total = detail.totalOf(selected)
    val night = detail.nightOf(selected)
    val deviceCount = detail.devices.size
    val label = if (selected == null) {
        if (deviceCount > 1) "全部 $deviceCount 台设备" else "全部设备"
    } else {
        detail.devices.firstOrNull { it.deviceKey == selected }?.deviceModel ?: "设备"
    }

    CardBox {
        Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(label, fontSize = 12.sp, color = Sub)
            Spacer(Modifier.height(6.dp))
            Text(fmtMinutes(total), fontSize = 30.sp, fontWeight = FontWeight.Bold)
            Spacer(Modifier.height(4.dp))
            Text(detail.date, fontSize = 11.5.sp, color = Sub)

            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("$night 分钟", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                    Text("夜间使用", fontSize = 11.5.sp, color = Sub)
                }
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    NightVerdictPill(detail.nightOk, detail.settled)
                    Spacer(Modifier.height(4.dp))
                    Text(
                        if (detail.settled) "得分 ${if (detail.points >= 0) "+" else ""}${detail.points}"
                        else "次日 21:00 结算",
                        fontSize = 11.5.sp, color = Sub,
                    )
                }
            }
        }
    }
}

/**
 * 设备切换：第一个是「全部」，其余是各台设备。
 *
 * 只有一台设备时不显示——一个按钮的选择没有意义，只会占屏幕。
 */
@Composable
internal fun DeviceChips(
    devices: List<MemberDeviceDetail>,
    selected: String?,
    onSelect: (String?) -> Unit,
) {
    if (devices.size < 2) return
    Row(
        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Chip("全部", selected == null) { onSelect(null) }
        devices.forEach { d ->
            Chip(d.deviceModel.ifBlank { "设备" }, selected == d.deviceKey) { onSelect(d.deviceKey) }
        }
    }
}

@Composable
private fun Chip(text: String, active: Boolean, onClick: () -> Unit) {
    Box(
        Modifier
            .clip(RoundedCornerShape(99.dp))
            .background(if (active) Primary else MaterialTheme.colorScheme.surface)
            .clickable { onClick() }
            .padding(horizontal = 14.dp, vertical = 7.dp),
    ) {
        Text(
            text,
            fontSize = 12.sp,
            fontWeight = if (active) FontWeight.SemiBold else FontWeight.Normal,
            color = if (active) Color.White else Sub,
        )
    }
}

/**
 * 24 小时柱状图。点柱子看该小时的具体分钟数。
 *
 * 高度按**当天最大值**归一化：不用固定刻度，否则孩子用 20 分钟的那天
 * 柱子只有 2 像素高，看不出形状。
 *
 * 坐标轴标签与柱子**共用同一套 24 等分网格**（都按 weight(1f) 分格）。
 * 之前用 SpaceBetween 均分 5 个标签，间距是「宽度 ÷ 4」，
 * 与「宽度 ÷ 24」的柱子网格根本不是一回事，必然错位。
 */
@Composable
internal fun HourlyBars(hours: List<Int>, nightStart: Int, nightEnd: Int) {
    val max = (hours.maxOrNull() ?: 0).coerceAtLeast(1)
    // 换设备/换天时数据变了，选中项要清空，避免指向一根已经不存在的柱子
    var picked by remember(hours) { mutableStateOf<Int?>(null) }
    val pickedMinutes = picked?.let { hours.getOrNull(it) ?: 0 } ?: 0

    CardBox {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("每小时", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.weight(1f))
            Text(
                picked?.let { "%02d:00–%02d:00 · %d 分钟".format(it, (it + 1) % 24, pickedMinutes) }
                    ?: "点柱子看详情",
                fontSize = 11.5.sp,
                color = if (picked == null) Sub else Primary,
                fontWeight = if (picked == null) FontWeight.Normal else FontWeight.SemiBold,
            )
        }
        Spacer(Modifier.height(12.dp))
        // 每根柱子与它的刻度写在**同一个格子**里（同一个 weight(1f)），
        // 这样刻度天然落在柱子正中——分成两行再各自均分，一定会错位。
        Row(Modifier.fillMaxWidth().height(122.dp)) {
            hours.forEachIndexed { h, v ->
                val isNight = isNightHour(h, nightStart, nightEnd)
                val selected = picked == h
                val hDp = when {
                    v <= 0 -> 2.dp
                    selected -> (10 + (v.toFloat() / max) * 82).dp
                    else -> (6 + (v.toFloat() / max) * 78).dp
                }
                Column(
                    Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .clickable { picked = if (selected) null else h },
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.BottomCenter) {
                        Box(
                            Modifier
                                .fillMaxWidth()
                                .height(hDp)
                                .clip(RoundedCornerShape(3.dp))
                                .background(
                                    when {
                                        v <= 0 -> Line
                                        selected -> Primary
                                        isNight -> Sub.copy(alpha = 0.7f)
                                        else -> Primary.copy(alpha = 0.72f)
                                    }
                                )
                        )
                    }
                    Spacer(Modifier.height(6.dp))
                    // 只在 0/6/12/18/23 写数字，其余留空占位；requiredWidth 防止窄格裁字
                    Text(
                        if (h in LABEL_HOURS) "$h" else "",
                        fontSize = 10.sp,
                        color = if (selected) Primary else Sub,
                        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
                        maxLines = 1,
                        softWrap = false,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.requiredWidth(30.dp).height(14.dp),
                    )
                }
            }
        }
        Text(
            "灰色柱子 = 夜间时段（$nightStart:00–$nightEnd:00）",
            fontSize = 11.sp, color = Sub,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

private val LABEL_HOURS = setOf(0, 6, 12, 18, 23)

private fun isNightHour(h: Int, start: Int, end: Int): Boolean {
    if (start == end) return false
    return if (start < end) h in start until end else (h >= start || h < end)
}

/** 近 7 天柱状图，最旧在左、今天在右。 */
@Composable
internal fun DailyBars(days: List<DayPoint>, values: List<Int>) {
    val max = (values.maxOrNull() ?: 0).coerceAtLeast(1)
    CardBox {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("近 7 天", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.weight(1f))
            Text("按天累计", fontSize = 11.5.sp, color = Sub)
        }
        Spacer(Modifier.height(12.dp))
        Row(
            Modifier.fillMaxWidth().height(132.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.Bottom,
        ) {
            days.forEachIndexed { i, d ->
                val v = values.getOrElse(i) { 0 }
                val hDp = if (v <= 0) 2.dp else (6 + (v.toFloat() / max) * 66).dp
                val isToday = i == days.lastIndex
                Column(
                    Modifier.weight(1f),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(
                        if (v >= 60) "${v / 60}h${v % 60}m" else "${v}m",
                        fontSize = 9.5.sp,
                        color = Sub,
                        maxLines = 1,
                    )
                    Spacer(Modifier.height(4.dp))
                    Box(
                        Modifier
                            .fillMaxWidth()
                            .height(hDp)
                            .clip(RoundedCornerShape(4.dp))
                            .background(if (v <= 0) Line else if (isToday) Primary else PrimarySoft)
                    )
                    Spacer(Modifier.height(4.dp))
                    Text(
                        d.date.takeLast(5),
                        fontSize = 9.sp,
                        color = if (isToday) Primary else Sub,
                        maxLines = 1,
                    )
                }
            }
        }
    }
}

/** 各设备明细行（全部设备视角下展示，选单台设备时不需要重复）。 */
@Composable
internal fun DeviceBreakdown(devices: List<MemberDeviceDetail>) {
    if (devices.isEmpty()) return
    CardBox {
        Text("各设备", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
        Spacer(Modifier.height(8.dp))
        devices.forEach { d ->
            Row(
                Modifier.fillMaxWidth().padding(vertical = 5.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("📱", fontSize = 13.sp)
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(d.deviceModel.ifBlank { "设备" }, fontSize = 13.sp)
                    Text(
                        if (d.nightMin != null && d.nightMin > 0) "夜间 ${d.nightMin} 分钟" else "夜间 0 分钟",
                        fontSize = 11.sp, color = Sub,
                    )
                }
                Text(fmtMinutes(d.totalMin), fontSize = 13.sp, fontWeight = FontWeight.Medium)
            }
        }
    }
}
