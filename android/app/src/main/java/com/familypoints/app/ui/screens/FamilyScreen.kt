package com.familypoints.app.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Avatar
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

/**
 * 所在家庭（2026-11-06）。
 *
 * 两种形态：
 * - **已入组**：家庭名称、家庭码、成员列表、退出家庭；组长另有「家庭管理」入口。
 * - **未入组**：搜索家庭码 → 申请加入（**需组长审批**）；或自己创建一个家。
 *
 * 家庭码只读：8 位、唯一、建组时生成，**永不可更改**；能否被搜到由组长在家庭管理里切换。
 */
@Composable
fun FamilyScreen(vm: AppViewModel, nav: NavHostController?, onBack: (() -> Unit)? = null) {
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me
    val inFamily = me?.familyId != null

    val familyName by vm.familyName.collectAsStateWithLifecycle()
    val members by vm.members.collectAsStateWithLifecycle()
    val code by vm.inviteCode.collectAsStateWithLifecycle()
    val codeHidden by vm.codeHidden.collectAsStateWithLifecycle()
    val search by vm.familySearch.collectAsStateWithLifecycle()
    val myReqs by vm.myJoinRequests.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()

    var searchInput by remember { mutableStateOf("") }
    var newName by remember { mutableStateOf("") }
    var confirmLeave by remember { mutableStateOf(false) }

    LaunchedEffect(inFamily) {
        if (inFamily) {
            vm.loadOverview()
            vm.loadInviteCode()
        } else {
            vm.loadMyJoinRequests()
        }
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "所在家庭", onBack = onBack)

        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (inFamily) {
                item {
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(familyName.ifBlank { "—" }, fontWeight = FontWeight.Bold, fontSize = 18.sp)
                                Text(
                                    "共 ${members.size} 位成员",
                                    fontSize = 12.sp, color = Sub,
                                )
                            }
                            Pill(
                                if (me?.isLeader == true) "组长" else "成员",
                                if (me?.isLeader == true) Primary else Sub,
                                if (me?.isLeader == true) com.familypoints.app.ui.theme.PrimarySoft
                                else androidx.compose.material3.MaterialTheme.colorScheme.surfaceVariant,
                            )
                        }
                    }
                }

                item {
                    CardBox {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("家庭码", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Spacer(Modifier.weight(1f))
                            if (codeHidden) Pill("已隐藏", Sub, Line)
                        }
                        Spacer(Modifier.height(8.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                code ?: "加载中…",
                                fontSize = 22.sp,
                                fontWeight = FontWeight.Bold,
                                color = Primary,
                                letterSpacing = 3.sp,
                                modifier = Modifier.weight(1f),
                            )
                            val clip = LocalClipboardManager.current
                            OutlinedButton(
                                onClick = {
                                    code?.let {
                                        clip.setText(AnnotatedString(it))
                                        vm.showMessage("家庭码已复制")
                                    }
                                },
                                modifier = Modifier.height(34.dp),
                            ) { Text("复制", fontSize = 12.5.sp, color = Sub) }
                        }
                        Text(
                            if (codeHidden) "已隐藏：别人搜不到，也无法凭码申请加入。"
                            else "别人可用它在「搜索家庭」里找到本组并申请加入，需要组长审批。",
                            fontSize = 11.5.sp, color = Sub,
                            modifier = Modifier.padding(top = 8.dp),
                        )
                    }
                }

                item {
                    CardBox {
                        Text("家庭成员", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                    }
                }
                if (members.isEmpty()) {
                    item { EmptyState("还没有成员") }
                } else {
                    items(members, key = { "fm-${it.id}" }) { m ->
                        CardBox {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Avatar(m.avatar, size = 38)
                                Spacer(Modifier.width(10.dp))
                                Column(Modifier.weight(1f)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Text(m.nickname, fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                        Spacer(Modifier.width(6.dp))
                                        // 职位贴在昵称旁边，而不是占右侧位置
                                        if (m.role == "leader") {
                                            Pill("组长", Primary, com.familypoints.app.ui.theme.PrimarySoft)
                                        }
                                        // 本人额外标一个「我」，避免在名单里找自己
                                        if (m.id == me?.id) {
                                            Spacer(Modifier.width(4.dp))
                                            Pill("我", Sub, Line)
                                        }
                                    }
                                    Text("用户名 ${m.username}", fontSize = 11.5.sp, color = Sub)
                                }
                                Text(
                                    "${m.score}",
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 15.sp,
                                )
                                Spacer(Modifier.width(3.dp))
                                Text("分", fontSize = 11.sp, color = Sub)
                            }
                        }
                    }
                }

                if (me?.isLeader == true) {
                    item {
                        Button(
                            onClick = { nav?.navigate(com.familypoints.app.ui.nav.Routes.MANAGE_MEMBERS) },
                            modifier = Modifier.fillMaxWidth().height(44.dp),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = com.familypoints.app.ui.theme.PrimarySoft,
                                contentColor = Primary,
                            ),
                        ) { Text("家庭管理（改组名 / 邀请 / 审批 / 踢人）", fontSize = 13.sp) }
                    }
                }

                // 组长后端明确禁止直接退出（必须先转让），所以不给他一个必然失败的按钮，
                // 只留说明。2026-11-06 反馈的「点退出报 HTTP 400」正是这个原因。
                if (me?.isLeader == true) {
                    item {
                        CardBox {
                            Text("组长不能直接退出", fontWeight = FontWeight.Medium, fontSize = 13.5.sp)
                            Text(
                                "需先把组长转让给其他成员，否则组内会没人能管理。转让入口在「家庭管理」。",
                                fontSize = 11.5.sp, color = Sub, lineHeight = 17.sp,
                                modifier = Modifier.padding(top = 4.dp),
                            )
                        }
                    }
                } else {
                    item {
                        OutlinedButton(
                            onClick = { confirmLeave = true },
                            modifier = Modifier.fillMaxWidth().height(44.dp),
                        ) { Text("退出家庭", color = Red, fontSize = 13.sp) }
                    }
                }
            } else {
                // ---------------- 未入组 ----------------
                item {
                    CardBox {
                        Text("还没有加入家庭", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                        Text(
                            "用家庭码搜索并申请加入（需要组长审批），或者自己创建一个家。",
                            fontSize = 12.sp, color = Sub, lineHeight = 18.sp,
                            modifier = Modifier.padding(top = 6.dp),
                        )
                    }
                }

                item {
                    CardBox {
                        Text("搜索家庭", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Spacer(Modifier.height(8.dp))
                        OutlinedTextField(
                            value = searchInput,
                            onValueChange = { searchInput = it.uppercase() },
                            label = { Text("8 位家庭码") },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Spacer(Modifier.height(10.dp))
                        Button(
                            onClick = { vm.searchFamily(searchInput) },
                            enabled = !busy && searchInput.isNotBlank(),
                            modifier = Modifier.fillMaxWidth().height(44.dp),
                            colors = ButtonDefaults.buttonColors(containerColor = Primary),
                        ) { Text("搜索") }
                    }
                }

                val s = search
                if (s != null) {
                    item {
                        CardBox {
                            if (!s.found) {
                                Text("没有找到这个家庭", fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                Text(
                                    "家庭码不存在，或该家庭已把家庭码设为隐藏。",
                                    fontSize = 11.5.sp, color = Sub,
                                    modifier = Modifier.padding(top = 4.dp),
                                )
                            } else {
                                Text(s.name, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                Text("${s.memberCount} 位成员", fontSize = 12.sp, color = Sub)
                                Spacer(Modifier.height(10.dp))
                                Button(
                                    onClick = { vm.requestJoin(searchInput) { vm.clearFamilySearch() } },
                                    enabled = !busy,
                                    modifier = Modifier.fillMaxWidth().height(42.dp),
                                    colors = ButtonDefaults.buttonColors(containerColor = Primary),
                                ) { Text("申请加入") }
                                Text(
                                    "提交后需要组长同意才会入组。",
                                    fontSize = 11.sp, color = Sub,
                                    modifier = Modifier.padding(top = 6.dp),
                                )
                            }
                        }
                    }
                }

                if (myReqs.isNotEmpty()) {
                    item {
                        CardBox {
                            Text("我的申请", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        }
                    }
                    items(myReqs, key = { "req-${it.id}" }) { r ->
                        CardBox {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text(r.familyName.ifBlank { "家庭" }, fontWeight = FontWeight.Medium, fontSize = 14.sp)
                                    Text("申请时间 ${r.createdAt.take(10)}", fontSize = 11.5.sp, color = Sub)
                                }
                                when (r.status) {
                                    "pending" -> Pill("等待审批", Sub, Line)
                                    "accepted" -> Pill("已通过", Green, GreenSoft)
                                    "rejected" -> Pill("已拒绝", Red, androidx.compose.ui.graphics.Color(0xFFFFEAEA))
                                    else -> Pill(r.statusText, Sub, Line)
                                }
                            }
                        }
                    }
                }

                item {
                    CardBox {
                        Text("创建家庭", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Text(
                            "创建后你就是组长，系统会自动生成一个家庭码。",
                            fontSize = 11.5.sp, color = Sub,
                            modifier = Modifier.padding(top = 4.dp),
                        )
                        Spacer(Modifier.height(8.dp))
                        OutlinedTextField(
                            value = newName,
                            onValueChange = { newName = it },
                            label = { Text("家庭名称") },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Spacer(Modifier.height(10.dp))
                        Button(
                            onClick = { vm.createFamily(newName) { newName = "" } },
                            enabled = !busy && newName.isNotBlank(),
                            modifier = Modifier.fillMaxWidth().height(44.dp),
                            colors = ButtonDefaults.buttonColors(containerColor = Primary),
                        ) { Text("创建家庭") }
                    }
                }
            }

            if (nav == null) {
                // 未入组时这个页面就是根页面，没有返回栈；给一个退出口，避免卡死
                item {
                    androidx.compose.material3.TextButton(
                        onClick = { vm.logout() },
                        modifier = Modifier.fillMaxWidth(),
                    ) { Text("退出登录", color = Sub, fontSize = 13.sp) }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (confirmLeave) {
        AlertDialog(
            onDismissRequest = { confirmLeave = false },
            title = { Text("退出家庭？") },
            text = {
                Text(
                    "退出后你在本组的积分会清零，且需要重新申请才能加入。",
                    fontSize = 14.sp, color = Sub,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmLeave = false
                    vm.leaveFamily()
                }) { Text("退出", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = {
                TextButton(onClick = { confirmLeave = false }) { Text("取消", color = Sub) }
            },
        )
    }
}
