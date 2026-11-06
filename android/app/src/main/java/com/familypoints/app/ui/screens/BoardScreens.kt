package com.familypoints.app.ui.screens

import androidx.compose.foundation.Canvas
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Divider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.data.api.MemberBrief
import com.familypoints.app.data.api.RankingRow
import com.familypoints.app.data.api.RecordView
import com.familypoints.app.data.api.TrendPoint
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.PointsText
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.SegmentedRow
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.nav.Routes
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Sub

@Composable
fun BoardScreen(vm: AppViewModel, nav: NavHostController) {
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val familyName by vm.familyName.collectAsStateWithLifecycle()
    val ranking by vm.ranking.collectAsStateWithLifecycle()
    val members by vm.members.collectAsStateWithLifecycle()
    val trend by vm.trend.collectAsStateWithLifecycle()

    var type by remember { mutableStateOf("points") }
    var period by remember { mutableStateOf("week") }

    LaunchedEffect(type, period) {
        vm.loadOverview()
        vm.loadRanking(type, period)
        vm.loadTrend(period)
    }

    Column(Modifier.fillMaxSize()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                familyName.ifBlank { "家庭积分" },
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.weight(1f),
            )
            if (me != null) {
                Pill("可用 ${me.score}", Primary, PrimarySoft)
            }
        }

        SegmentedRow(
            options = listOf("积分榜" to (type == "points"), "得分榜" to (type == "score")),
        ) { index -> type = if (index == 0) "points" else "score" }

        // 本周/本月同时作用于得分榜与趋势折线；积分榜是净额，后端会忽略该参数。
        SegmentedRow(
            options = listOf("本周" to (period == "week"), "本月" to (period == "month")),
        ) { index -> period = if (index == 0) "week" else "month" }

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    if (ranking.isEmpty()) {
                        EmptyState("暂无数据")
                    } else {
                        ranking.forEachIndexed { index, row ->
                            RankingItem(
                                index = index,
                                row = row,
                                type = type,
                                isMe = row.memberId == me?.id,
                                onClick = { nav.navigate(Routes.memberDetail(row.memberId)) },
                            )
                            if (index != ranking.lastIndex) Divider(color = Line)
                        }
                    }
                }
            }
            item { TrendCard(points = trend, period = period) }
            item {
                CardBox {
                    Text(
                        if (type == "points") {
                            "积分榜 = 剩余积分（净额）。兑换、赠送、任务收支都已结算进余额。"
                        } else {
                            "得分榜 = 区间内赚到的分（毛额）。只累加正分，不看花掉的。"
                        },
                        fontSize = 12.sp,
                        color = Sub,
                        lineHeight = 18.sp,
                    )
                }
            }
            item { Spacer(Modifier.height(10.dp)) }
        }
    }
}

@Composable
private fun RankingItem(
    index: Int,
    row: RankingRow,
    type: String,
    isMe: Boolean,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onClick() }
            .padding(vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.width(30.dp), contentAlignment = Alignment.Center) {
            Text(
                text = when (index) {
                    0 -> "🥇"
                    1 -> "🥈"
                    2 -> "🥉"
                    else -> "${index + 1}"
                },
                fontSize = if (index < 3) 19.sp else 15.sp,
                fontWeight = FontWeight.Bold,
                color = Sub,
            )
        }
        Avatar(row.avatar)
        Spacer(Modifier.width(11.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(row.nickname, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                if (isMe) {
                    Spacer(Modifier.width(6.dp))
                    Pill("我", Primary, PrimarySoft)
                }
            }
            Text(
                text = if (type == "points") {
                    buildString {
                        append("可用 ${row.score}")
                        if (row.frozenScore > 0) append(" · 冻结 ${row.frozenScore}")
                    }
                } else {
                    buildString {
                        append("区间得分 ${row.periodScore}")
                        if (row.streak > 0) append(" · 🔥 连续 ${row.streak} 天")
                    }
                },
                fontSize = 12.sp,
                color = Sub,
            )
        }
        PointsText(
            if (type == "points") row.score + row.frozenScore else row.periodScore,
            big = true,
            // 积分榜是余额（不带符号），得分榜是区间所得（可以带 +）
            signed = type != "points",
        )
    }
}

@Composable
fun MemberDetailScreen(vm: AppViewModel, nav: NavHostController, memberId: Long) {
    val members by vm.members.collectAsStateWithLifecycle()
    val records by vm.records.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val member: MemberBrief? = members.firstOrNull { it.id == memberId }

    LaunchedEffect(memberId) {
        vm.loadOverview()
        vm.loadRecords(memberId = memberId)
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "成员详情", onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Column(
                        Modifier.fillMaxWidth(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Avatar(member?.avatar, size = 64)
                        Spacer(Modifier.height(10.dp))
                        Text(
                            member?.nickname ?: "成员",
                            style = MaterialTheme.typography.titleLarge,
                        )
                        Text(
                            buildString {
                                if (member?.isLeader == true) append("组长") else append("成员")
                                if (member?.id == me?.id) append(" · 我")
                            },
                            color = Sub,
                            fontSize = 12.sp,
                            modifier = Modifier.padding(top = 2.dp),
                        )
                        Spacer(Modifier.height(16.dp))
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceEvenly,
                        ) {
                            StatCell("可用", "${member?.score ?: 0}")
                            StatCell("冻结", "${member?.frozenScore ?: 0}", Blue)
                            StatCell("总计", "${member?.totalScore ?: 0}")
                        }
                    }
                }
            }
            item {
                CardBox {
                    Text("积分流水", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(6.dp))
                    if (records.isEmpty()) {
                        EmptyState("暂无记录")
                    } else {
                        records.take(50).forEachIndexed { i, r ->
                            RecordRow(r)
                            if (i != records.take(50).lastIndex) Divider(color = Line)
                        }
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}

@Composable
private fun StatCell(
    label: String,
    value: String,
    color: androidx.compose.ui.graphics.Color? = null,
) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text(
            value,
            fontSize = 21.sp,
            fontWeight = FontWeight.Bold,
            color = color ?: MaterialTheme.colorScheme.onSurface,
        )
        Text(label, fontSize = 12.sp, color = Sub)
    }
}

@Composable
fun RecordsScreen(vm: AppViewModel, nav: NavHostController) {
    val records by vm.records.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { vm.loadRecords() }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "积分记录", onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
        ) {
            item {
                CardBox {
                    if (records.isEmpty()) EmptyState("暂无记录")
                    else records.forEachIndexed { i, r ->
                        RecordRow(r)
                        if (i != records.lastIndex) Divider(color = Line)
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}

@Composable
private fun RecordRow(r: RecordView) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                "${r.memberName} · ${r.title}",
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
            )
            Text(fmtTime(r.occurredAt), fontSize = 11.5.sp, color = Sub)
        }
        PointsText(r.points)
    }
}

/** 积分趋势折线：只画后端 /api/app/stats/trend 的 date / earned / net，前端不做任何判定。 */
@Composable
private fun TrendCard(points: List<TrendPoint>, period: String) {
    CardBox {
        Text(
            "积分趋势（全组 · ${if (period == "month") "本月" else "本周"}）",
            fontWeight = FontWeight.SemiBold,
            fontSize = 14.sp,
        )
        Spacer(Modifier.height(4.dp))
        Text(
            "共赚 ${points.sumOf { it.earned }} 分 · 净 ${points.sumOf { it.net }} 分",
            fontSize = 11.5.sp,
            color = Sub,
        )
        Spacer(Modifier.height(10.dp))
        if (points.isEmpty()) {
            EmptyState("这段时间没有积分变动")
        } else {
            TrendChart(points)
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text(points.first().date.takeLast(5), fontSize = 10.sp, color = Sub)
                if (points.size > 2) {
                    Text(points[points.size / 2].date.takeLast(5), fontSize = 10.sp, color = Sub)
                }
                Text(points.last().date.takeLast(5), fontSize = 10.sp, color = Sub)
            }
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                LegendDot(Primary)
                Text(" 赚到的分 earned", fontSize = 11.sp, color = Sub)
                Spacer(Modifier.width(12.dp))
                LegendDot(Blue)
                Text(" 净变动 net", fontSize = 11.sp, color = Sub)
            }
        }
        Spacer(Modifier.height(8.dp))
        Text(
            "曲线只画服务端按 occurred_at 聚合的结果；补记的旧日期会落在它自己那天。",
            fontSize = 11.sp,
            color = Sub,
            lineHeight = 16.sp,
        )
    }
}

@Composable
private fun LegendDot(color: androidx.compose.ui.graphics.Color) {
    Box(
        Modifier
            .width(10.dp)
            .height(3.dp)
            .clip(RoundedCornerShape(2.dp))
            .background(color),
    )
}

@Composable
private fun TrendChart(points: List<TrendPoint>) {
    // net 可能为负（兑换 / 扣分），所以纵轴下界取 0 与最小净值的较小者。
    val maxV = maxOf(1, points.maxOf { maxOf(it.earned, it.net) })
    val minV = minOf(0, points.minOf { minOf(it.earned, it.net) })
    val span = (maxV - minV).coerceAtLeast(1).toFloat()

    Canvas(
        modifier = Modifier
            .fillMaxWidth()
            .height(150.dp)
            .padding(horizontal = 4.dp, vertical = 8.dp),
    ) {
        val left = 0f
        val right = size.width
        val bottom = size.height
        val h = bottom
        fun xAt(i: Int): Float =
            if (points.size == 1) (left + right) / 2 else left + (right - left) * i / (points.size - 1)
        fun yAt(v: Int): Float = bottom - (v - minV) / span * h

        listOf(0f, 0.5f, 1f).forEach { f ->
            val y = bottom - h * f
            drawLine(Line, Offset(left, y), Offset(right, y), strokeWidth = 1.dp.toPx())
        }
        if (minV < 0) {
            drawLine(
                Sub.copy(alpha = 0.5f),
                Offset(left, yAt(0)),
                Offset(right, yAt(0)),
                strokeWidth = 1.dp.toPx(),
            )
        }

        // 先画 net（虚线），再画 earned（实线），保证实线在上层。
        val dash = PathEffect.dashPathEffect(floatArrayOf(10f, 8f), 0f)
        if (points.size > 1) {
            points.forEachIndexed { i, p ->
                if (i > 0) {
                    drawLine(
                        Blue,
                        Offset(xAt(i - 1), yAt(points[i - 1].net)),
                        Offset(xAt(i), yAt(p.net)),
                        strokeWidth = 2.5.dp.toPx(),
                        pathEffect = dash,
                    )
                    drawLine(
                        Primary,
                        Offset(xAt(i - 1), yAt(points[i - 1].earned)),
                        Offset(xAt(i), yAt(p.earned)),
                        strokeWidth = 3.dp.toPx(),
                    )
                }
            }
        }
        points.forEachIndexed { i, p ->
            drawCircle(Blue, radius = 3.dp.toPx(), center = Offset(xAt(i), yAt(p.net)))
            drawCircle(Primary, radius = 3.5.dp.toPx(), center = Offset(xAt(i), yAt(p.earned)))
        }
    }
}

fun fmtTime(iso: String): String {
    if (iso.length < 16) return iso
    // 2026-11-03T21:04:00+08:00 → 11-03 21:04
    return runCatching {
        val date = iso.substring(5, 10)
        val time = iso.substring(11, 16)
        "$date $time"
    }.getOrDefault(iso)
}
