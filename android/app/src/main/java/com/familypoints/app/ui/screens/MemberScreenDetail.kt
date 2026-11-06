package com.familypoints.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.theme.Sub

/**
 * 家庭成员屏幕时间详情（2026-11-06）。
 *
 * 与「屏幕」tab 用同一套组件：总时长、24 小时、近 7 天、各设备。
 * 区别只是数据来源是**别人的**，所以没有「上传快照」这类写操作，纯只读。
 */
@Composable
fun MemberScreenDetailScreen(vm: AppViewModel, nav: NavHostController, memberId: Long) {
    val detail by vm.memberDetail.collectAsStateWithLifecycle()
    val rules by vm.screenRules.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()

    // 夜间时段按**被查看成员**的规则着色：不同成员可以有不同的睡觉时间点。
    // 拿不到该成员的规则时退回自己的，再退回 0–7（只是颜色，不影响任何数字）。
    val myId = (session as? Session.Ready)?.me?.id ?: 0L
    val rule = rules.firstOrNull { it.memberId == memberId }
        ?: rules.firstOrNull { it.memberId == myId }
        ?: rules.firstOrNull()
    val nightStart = rule?.nightStartHour ?: 0
    val nightEnd = rule?.nightEndHour ?: 7

    var pickedDevice by remember(memberId) { mutableStateOf<String?>(null) }

    LaunchedEffect(memberId) { vm.loadScreenDetail(memberId) }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(
            title = detail?.nickname?.let { "$it 的屏幕时间" } ?: "屏幕时间",
            onBack = { nav.popBackStack() },
        )

        val d = detail
        if (d == null) {
            Column(Modifier.fillMaxSize().padding(16.dp)) {
                CardBox { Text("正在读取…", fontSize = 13.sp, color = Sub) }
            }
            return@Column
        }

        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item { UsageTotalCard(d, pickedDevice) }
            if (d.devices.size > 1) {
                item { DeviceChips(d.devices, pickedDevice) { pickedDevice = it } }
            }
            item { HourlyBars(d.hourlyOf(pickedDevice), nightStart, nightEnd) }
            item { DailyBars(d.days, d.daysOf(pickedDevice)) }
            if (pickedDevice == null && d.devices.size > 1) {
                item { DeviceBreakdown(d.devices) }
            }
            if (d.devices.isEmpty()) {
                item {
                    CardBox {
                        Text("今天还没有上报数据", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Text(
                            "对方打开 App 或今晚 21:00 定时上报后，这里会出现明细。",
                            fontSize = 12.sp, color = Sub,
                            modifier = Modifier.padding(top = 6.dp),
                        )
                    }
                }
            }
            item {
                CardBox {
                    Text("口径说明", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    Spacer(Modifier.height(6.dp))
                    Text(
                        "总时长 = 该成员全部设备相加；切换设备可只看单台。\n" +
                            "判定与积分由服务端结算，未结算前显示「待结算」，不做推测。",
                        fontSize = 12.sp, color = Sub, lineHeight = 18.sp,
                    )
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}
