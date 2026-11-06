package com.familypoints.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Sub

/** 预设头像：家庭场景不做文件上传，用 emoji 标识足够且零成本。 */
private val AVATARS = listOf("😀", "🐻", "🐱", "🐶", "🦊", "🐼", "🌟", "🌸", "⚽️", "🎈")

/**
 * 个人设置页（2026-11-05 新增）。
 *
 * 从「我的」页点头像进来；可改昵称、头像、密码。username 永不可改。
 * **不做本地校验**（长度等一律交给后端），只校验「两次输入是否一致」这类纯表单问题。
 */
@Composable
fun ProfileEditScreen(vm: AppViewModel, nav: NavHostController) {
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    var nickname by remember(me?.id) { mutableStateOf(me?.nickname.orEmpty()) }
    var avatar by remember(me?.id) { mutableStateOf(me?.avatar.orEmpty().ifBlank { AVATARS.first() }) }

    var oldPwd by remember { mutableStateOf("") }
    var newPwd by remember { mutableStateOf("") }
    var againPwd by remember { mutableStateOf("") }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "个人设置", onBack = { nav.popBackStack() })

        Column(
            Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            // 用户名放最上面：它是登录凭证，**不可修改**，先讲清楚再让人去改昵称，
            // 免得用户把「改昵称」误当成「改用户名」。
            CardBox {
                Text("用户名", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                Spacer(Modifier.height(8.dp))
                Row(
                    Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        me?.username.orEmpty().ifBlank { "—" },
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier.weight(1f),
                    )
                    Text("不可修改", fontSize = 11.5.sp, color = Sub)
                }
                Text(
                    "用户名用于登录，注册后不能更改；下面可以改的是昵称和头像。",
                    fontSize = 11.5.sp, color = Sub,
                    modifier = Modifier.padding(top = 6.dp),
                )
            }

            CardBox {
                Text("头像", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                Spacer(Modifier.height(10.dp))
                // 两行五列，点选即高亮
                AVATARS.chunked(5).forEach { row ->
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        row.forEach { a ->
                            val on = a == avatar
                            Box(
                                Modifier
                                    .size(48.dp)
                                    .clip(RoundedCornerShape(14.dp))
                                    .background(
                                        if (on) MaterialTheme.colorScheme.surfaceVariant else Color.Transparent
                                    )
                                    .border(
                                        width = if (on) 2.dp else 1.dp,
                                        color = if (on) Primary else Line,
                                        shape = RoundedCornerShape(14.dp),
                                    )
                                    .clickable { avatar = a },
                                contentAlignment = Alignment.Center,
                            ) { Text(a, fontSize = 22.sp) }
                        }
                    }
                }
            }

            CardBox {
                Text("昵称", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = nickname,
                    onValueChange = { nickname = it },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "昵称会显示在积分榜与流水里；历史记录里的名字是快照，不会被改。",
                    fontSize = 11.5.sp, color = Sub,
                    modifier = Modifier.padding(top = 6.dp),
                )
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = { vm.updateProfile(nickname.trim(), avatar) { nav.popBackStack() } },
                    modifier = Modifier.fillMaxWidth().height(46.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = Primary, contentColor = Color.White),
                ) { Text("保存资料") }
            }

            CardBox {
                Text("修改密码", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = oldPwd,
                    onValueChange = { oldPwd = it },
                    label = { Text("原密码") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = newPwd,
                    onValueChange = { newPwd = it },
                    label = { Text("新密码（至少 8 位）") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = againPwd,
                    onValueChange = { againPwd = it },
                    label = { Text("再输一次") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = {
                        // 只做「两次一致」这种表单校验；长度/强度由后端判定
                        if (newPwd != againPwd) {
                            vm.showMessage("两次输入的新密码不一致")
                            return@Button
                        }
                        vm.changePassword(oldPwd, newPwd) {
                            oldPwd = ""; newPwd = ""; againPwd = ""
                        }
                    },
                    modifier = Modifier.fillMaxWidth().height(46.dp),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.surfaceVariant,
                        contentColor = MaterialTheme.colorScheme.onSurface,
                    ),
                ) { Text("修改密码") }
                Text(
                    "改密后其他设备会被登出，需要用新密码重新登录。",
                    fontSize = 11.5.sp, color = Sub,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            Spacer(Modifier.height(16.dp))
        }
    }
}
