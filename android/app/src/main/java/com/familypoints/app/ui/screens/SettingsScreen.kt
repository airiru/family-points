package com.familypoints.app.ui.screens

import android.content.Intent
import android.net.Uri
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
import androidx.compose.material3.Divider
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.BuildConfig
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.nav.Routes
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

/** 协议与清单文档（开发阶段为占位文案，正式法务文案上线前替换）。 */
val AGREEMENTS = listOf(
    "privacy" to ("隐私协议" to
        "本应用（家庭积分）隐私协议（占位文本，正式文案待补充）。\n\n" +
        "1. 我们收集的信息：账号用户名、昵称、头像标识；屏幕使用时长统计（按小时分桶，仅在本家庭组内展示）；设备型号与自生成的设备标识。\n" +
        "2. 信息用途：仅用于向本家庭组成员展示积分与屏幕时间数据。\n" +
        "3. 信息存储：存储于运营方自建服务器；不向任何第三方提供。\n" +
        "4. 你可以随时注销账号，注销后账号删除、设备绑定释放，历史积分流水保留。"),
    "user" to ("用户协议" to
        "本应用（家庭积分）用户协议（占位文本，正式文案待补充）。\n\n" +
        "1. 本应用面向家庭场景，提供积分记录、任务协作与屏幕时间统计。\n" +
        "2. 注册即表示同意本协议；账号仅限本人使用，用户名注册后不可修改。\n" +
        "3. 禁止利用本应用进行任何违反法律法规的活动。"),
    "collect" to ("个人信息收集清单" to
        "个人信息收集清单（占位文本，正式文案待补充）。\n\n" +
        "· 账号信息：用户名、密码（仅存 bcrypt 哈希）、昵称、头像标识\n" +
        "· 使用数据：每日屏幕使用总时长、夜间时段使用时长、按小时分布\n" +
        "· 设备信息：设备型号、自生成设备标识（ANDROID_ID）\n" +
        "· 上述信息仅用于本应用功能本身。"),
    "thirdparty" to ("第三方个人信息收集清单" to
        "第三方个人信息收集清单（占位文本，正式文案待补充）。\n\n" +
        "本应用不接入任何第三方统计、广告或推送 SDK，不向第三方提供任何个人信息。"),
)

/** 设置二级页（2026-11-07）：协议/清单、版本、官网 + 会话（退出登录 / 注销账号）。 */
@Composable
fun SettingsScreen(vm: AppViewModel, nav: NavHostController) {
    val context = LocalContext.current
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    var showCancel by remember { mutableStateOf(false) }
    var cancelPwd by remember { mutableStateOf("") }
    var showUpdate by remember { mutableStateOf(false) }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "设置", onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text("协议与清单", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    AGREEMENTS.forEachIndexed { i, (route, titlePair) ->
                        if (i > 0) Divider(color = Line)
                        MenuItem("📄", titlePair.first, "") { nav.navigate(Routes.agreement(route)) }
                    }
                }
            }

            item {
                CardBox {
                    Text("关于", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    // 当前版本（只读行）
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 10.dp),
                    ) {
                        Text("当前版本", fontWeight = FontWeight.Medium, fontSize = 14.sp, modifier = Modifier.weight(1f))
                        Text("v${BuildConfig.VERSION_NAME}", fontSize = 13.sp, color = Sub)
                    }
                    Divider(color = Line)
                    MenuItem("⬆️", "版本更新", "检查是否有新版本") { showUpdate = true }
                    Divider(color = Line)
                    MenuItem("🌐", "官网", "airiru.site") {
                        runCatching {
                            context.startActivity(
                                Intent(Intent.ACTION_VIEW, Uri.parse("https://airiru.site"))
                                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                            )
                        }
                    }
                }
            }

            // ---------- 会话（从个人设置移入，2026-11-07） ----------
            item {
                CardBox {
                    Text("会话", fontSize = 12.sp, color = Sub, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    Button(
                        onClick = {
                            vm.logout()
                            nav.popBackStack()
                        },
                        modifier = Modifier.fillMaxWidth().height(46.dp),
                    ) { Text("退出登录") }
                    Spacer(Modifier.height(10.dp))
                    OutlinedButton(
                        onClick = { showCancel = true },
                        modifier = Modifier.fillMaxWidth().height(46.dp),
                    ) { Text("注销账号", color = Red, fontSize = 13.sp) }
                    Text(
                        "注销后账号即从系统中删除，历史积分流水保留；在家庭组内时需先退出。",
                        fontSize = 11.sp, color = Sub,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (showUpdate) {
        AlertDialog(
            onDismissRequest = { showUpdate = false },
            title = { Text("版本更新") },
            text = { Text("当前已是最新版本（v${BuildConfig.VERSION_NAME}）。", fontSize = 14.sp, color = Sub) },
            confirmButton = {
                TextButton(onClick = { showUpdate = false }) { Text("知道了", color = Primary) }
            },
        )
    }

    if (showCancel) {
        AlertDialog(
            onDismissRequest = { showCancel = false },
            title = { Text("注销账号？") },
            text = {
                Column {
                    Text(
                        "注销后无法恢复：账号删除、登录态全部失效、设备绑定释放；历史积分流水保留。在家庭组内时需先退出家庭组。",
                        fontSize = 13.sp, color = Sub,
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedTextField(
                        value = cancelPwd,
                        onValueChange = { cancelPwd = it },
                        label = { Text("输入密码确认") },
                        visualTransformation = androidx.compose.ui.text.input.PasswordVisualTransformation(),
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.cancelAccount(cancelPwd) {
                        cancelPwd = ""
                        showCancel = false
                        nav.popBackStack()
                    }
                }) { Text("注销", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = {
                TextButton(onClick = { showCancel = false; cancelPwd = "" }) { Text("取消", color = Sub) }
            },
        )
    }
}

/** 协议 / 清单文档二级页（占位文案）。 */
@Composable
fun AgreementScreen(vm: AppViewModel, nav: NavHostController, type: String) {
    val (title, body) = AGREEMENTS.firstOrNull { it.first == type }?.second
        ?: ("协议" to "内容不存在")
    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = title, onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text(
                        body,
                        fontSize = 13.sp,
                        lineHeight = 20.sp,
                        modifier = Modifier.padding(14.dp),
                    )
                }
            }
        }
    }
}
