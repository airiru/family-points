package com.familypoints.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.data.api.MemberDeviceView
import com.familypoints.app.data.api.UnbindRequestView
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub
import com.familypoints.app.util.DeviceInfo

/**
 * 设备管理（2026-11-07，docs 15.5）：
 * 显示本账号绑定的设备；组长可直接解绑任何设备，
 * 普通成员解绑需发起申请、组长审批（设备管理页内联审批）。
 */
@Composable
fun DeviceManageScreen(vm: AppViewModel, nav: NavHostController) {
    val context = androidx.compose.ui.platform.LocalContext.current
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val isLeader = me?.isLeader == true

    val devices by vm.myDevices.collectAsStateWithLifecycle()
    val mine by vm.myUnbindRequests.collectAsStateWithLifecycle()
    val pending by vm.pendingUnbindRequests.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { vm.loadDeviceManage() }

    // 待确认解绑的设备 key；null = 不弹
    var confirmTarget by remember { mutableStateOf<MemberDeviceView?>(null) }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "设备管理", onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Column(Modifier.padding(14.dp)) {
                        Text("绑定的设备", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                        Text(
                            "一台设备只能绑定一个账号；解绑后可在其他账号上重新绑定。" +
                                if (isLeader) "组长可以直接解绑任何设备。"
                                else "解绑需发起申请，由组长审批。",
                            fontSize = 12.sp, color = Sub, lineHeight = 18.sp,
                            modifier = Modifier.padding(top = 6.dp),
                        )
                    }
                }
            }

            if (devices.isEmpty()) {
                item {
                    CardBox {
                        Text(
                            "还没有绑定任何设备\n设备首次上报屏幕时间时会自动绑定到当前账号",
                            fontSize = 13.sp, color = Sub, lineHeight = 19.sp,
                        )
                    }
                }
            }
            items(devices.size) { i ->
                val d = devices[i]
                val isCurrent = d.device_key == DeviceInfo.key(context)
                val req = mine.firstOrNull { it.device_key == d.device_key && it.status == "pending" }
                CardBox {
                    Column(Modifier.padding(14.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Text(
                                        d.device_model.ifBlank { "未知设备" },
                                        fontWeight = FontWeight.SemiBold, fontSize = 14.sp,
                                    )
                                    Spacer(Modifier.padding(start = 6.dp))
                                    if (isCurrent) Pill("本机", Primary, com.familypoints.app.ui.theme.PrimarySoft)
                                }
                                Text(
                                    "绑定于 ${d.first_seen_at.take(10)} · 最近上报 ${d.last_seen_at.take(16).replace('T', ' ')}",
                                    fontSize = 11.5.sp, color = Sub,
                                    modifier = Modifier.padding(top = 4.dp),
                                )
                                if (req != null) {
                                    Text(
                                        "解绑申请审批中（${req.created_at.take(10)} 提交）",
                                        fontSize = 11.5.sp,
                                        color = com.familypoints.app.ui.theme.Primary,
                                        modifier = Modifier.padding(top = 4.dp),
                                    )
                                }
                            }
                            Pill("已绑定", Green, GreenSoft)
                        }
                        Divider(color = Line, modifier = Modifier.padding(vertical = 10.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Spacer(Modifier.weight(1f))
                            if (req != null) {
                                TextButton(onClick = { vm.cancelUnbind(req.id) }) {
                                    Text("撤回申请", color = Sub, fontSize = 13.sp)
                                }
                            } else {
                                TextButton(onClick = { confirmTarget = d }) {
                                    Text("解除绑定", color = Red, fontSize = 13.sp)
                                }
                            }
                        }
                    }
                }
            }

            // 组长：本组待审批的解绑申请
            if (isLeader) {
                item {
                    Text(
                        "待审批的解绑申请",
                        fontWeight = FontWeight.SemiBold, fontSize = 14.sp,
                        modifier = Modifier.padding(top = 6.dp),
                    )
                }
                if (pending.isEmpty()) {
                    item {
                        CardBox { Text("暂无待审批申请", fontSize = 13.sp, color = Sub) }
                    }
                }
                items(pending.size) { i ->
                    val r = pending[i]
                    CardBox {
                        Column(Modifier.padding(14.dp)) {
                            Text(
                                "${r.member_nickname}（${r.member_username}）",
                                fontWeight = FontWeight.SemiBold, fontSize = 14.sp,
                            )
                            Text(
                                "申请解绑「${r.device_model.ifBlank { r.device_key.take(8) }}」· ${r.created_at.take(16).replace('T', ' ')}",
                                fontSize = 12.sp, color = Sub,
                                modifier = Modifier.padding(top = 4.dp),
                            )
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.padding(top = 8.dp),
                            ) {
                                Spacer(Modifier.weight(1f))
                                TextButton(onClick = { vm.decideUnbind(r.id, false) }) {
                                    Text("拒绝", color = Sub, fontSize = 13.sp)
                                }
                                OutlinedButton(
                                    onClick = { vm.decideUnbind(r.id, true) },
                                    modifier = Modifier.padding(start = 4.dp),
                                ) { Text("同意解绑", color = Primary, fontSize = 13.sp) }
                            }
                        }
                    }
                }
            }

            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    confirmTarget?.let { d ->
        AlertDialog(
            onDismissRequest = { confirmTarget = null },
            title = { Text("解除绑定？") },
            text = {
                Text(
                    if (isLeader)
                        "解绑「${d.device_model.ifBlank { "该设备" }}」后，它将不再关联任何账号，下次上报时会绑定到当时登录的账号。"
                    else
                        "将向组长申请解绑「${d.device_model.ifBlank { "该设备" }}」，审批通过后生效。",
                    fontSize = 14.sp, color = Sub,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val key = d.device_key
                    confirmTarget = null
                    if (isLeader) vm.unbindDevice(key) else vm.requestUnbindDevice(key)
                }) { Text(if (isLeader) "解绑" else "提交申请", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = {
                TextButton(onClick = { confirmTarget = null }) { Text("取消", color = Sub) }
            },
        )
    }
}
