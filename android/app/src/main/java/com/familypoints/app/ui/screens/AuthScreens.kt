package com.familypoints.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.familypoints.app.data.api.MeView
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Sub

@Composable
fun LoginScreen(vm: AppViewModel, onGoRegister: () -> Unit) {
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    val busy by vm.busy.collectAsStateWithLifecycle()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp),
    ) {
        Spacer(Modifier.height(72.dp))
        Text("🏆", fontSize = 52.sp, modifier = Modifier.fillMaxWidth(), textAlign = TextAlign.Center)
        Spacer(Modifier.height(14.dp))
        Text(
            "家庭积分",
            style = MaterialTheme.typography.headlineMedium,
            modifier = Modifier.fillMaxWidth(),
            textAlign = TextAlign.Center,
        )
        Text(
            "和家人一起，把好习惯攒成分",
            color = Sub,
            fontSize = 13.sp,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 6.dp),
            textAlign = TextAlign.Center,
        )
        Spacer(Modifier.height(34.dp))

        CardBox {
            OutlinedTextField(
                value = username,
                onValueChange = { username = it },
                label = { Text("用户名") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(10.dp))
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("密码") },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(18.dp))
            Button(
                onClick = { vm.login(username, password) },
                enabled = !busy && username.isNotBlank() && password.isNotBlank(),
                modifier = Modifier
                    .fillMaxWidth()
                    .height(48.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Primary),
            ) {
                if (busy) CircularProgressIndicator(Modifier.height(18.dp), strokeWidth = 2.dp)
                else Text("登录", fontWeight = FontWeight.SemiBold)
            }
            Spacer(Modifier.height(8.dp))
            TextButton(onClick = onGoRegister, modifier = Modifier.fillMaxWidth()) {
                Text("还没有账号？注册", color = Primary, fontSize = 13.sp)
            }
        }

        Spacer(Modifier.height(20.dp))
        Text(
            "账号只能本人注册。加入家庭组需要组长提供的邀请码。",
            color = Sub,
            fontSize = 12.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
fun RegisterScreen(vm: AppViewModel, onBack: () -> Unit) {
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var nickname by remember { mutableStateOf("") }
    var familyName by remember { mutableStateOf("") }
    var createFamily by remember { mutableStateOf(true) }
    val busy by vm.busy.collectAsStateWithLifecycle()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp),
    ) {
        Spacer(Modifier.height(40.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("‹ 返回", fontSize = 15.sp) }
        }
        Text("注册", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(20.dp))

        CardBox {
            OutlinedTextField(
                value = username,
                onValueChange = { username = it },
                label = { Text("用户名（全局唯一）") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(10.dp))
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                // 不在这里写长度规则：密码策略只由后端一处定义，前端只展示后端返回的错误
                label = { Text("密码") },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(10.dp))
            OutlinedTextField(
                value = nickname,
                onValueChange = { nickname = it },
                label = { Text("昵称（在家庭里显示的名字）") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(16.dp))

            Text("接下来", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.height(10.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                SegChip("创建家庭组", createFamily) { createFamily = true }
                SegChip("稍后加入", !createFamily) { createFamily = false }
            }

            if (createFamily) {
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = familyName,
                    onValueChange = { familyName = it },
                    label = { Text("家庭组名称") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "创建者自动成为该组组长，但不获得任何后台权限。",
                    color = Sub,
                    fontSize = 11.5.sp,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }

            Spacer(Modifier.height(18.dp))
            Button(
                onClick = { vm.register(username, password, nickname, createFamily, familyName) },
                enabled = !busy && username.isNotBlank() && password.length >= 4,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(48.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Primary),
            ) {
                Text(if (createFamily) "注册并创建家庭组" else "注册", fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

@Composable
private fun SegChip(text: String, selected: Boolean, onClick: () -> Unit) {
    if (selected) {
        Button(
            onClick = onClick,
            colors = ButtonDefaults.buttonColors(containerColor = Primary),
            modifier = Modifier.height(38.dp),
        ) { Text(text, fontSize = 13.sp) }
    } else {
        OutlinedButton(onClick = onClick, modifier = Modifier.height(38.dp)) {
            Text(text, fontSize = 13.sp, color = Sub)
        }
    }
}

/** 已登录但未加入任何家庭组时的页面。 */
@Composable
fun JoinFamilyScreen(vm: AppViewModel, me: MeView) {
    var code by remember { mutableStateOf("") }
    var confirmLeave by remember { mutableStateOf(false) }
    val busy by vm.busy.collectAsStateWithLifecycle()
    val myInvitations by vm.myInvitations.collectAsStateWithLifecycle()
    val pending = myInvitations.filter { it.isPending }

    LaunchedEffect(Unit) { vm.loadMyInvitations() }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp),
    ) {
        Spacer(Modifier.height(60.dp))
        Text("👋", fontSize = 44.sp, modifier = Modifier.fillMaxWidth(), textAlign = TextAlign.Center)
        Spacer(Modifier.height(12.dp))
        Text(
            "你好，${me.nickname.ifBlank { "新成员" }}",
            style = MaterialTheme.typography.headlineMedium,
            modifier = Modifier.fillMaxWidth(),
            textAlign = TextAlign.Center,
        )
        Text(
            "你还没有加入家庭组",
            color = Sub,
            fontSize = 13.sp,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 6.dp),
            textAlign = TextAlign.Center,
        )
        Spacer(Modifier.height(32.dp))

        CardBox {
            Text("输入邀请码", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.height(10.dp))
            OutlinedTextField(
                value = code,
                onValueChange = { code = it.uppercase() },
                label = { Text("8 位邀请码") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Text(
                "邀请码由组长生成，有效期 7 天。被踢出或自行退出的家庭组无法再次加入。",
                color = Sub,
                fontSize = 11.5.sp,
                modifier = Modifier.padding(top = 8.dp),
            )
            Spacer(Modifier.height(16.dp))
            Button(
                onClick = { vm.joinFamily(code) },
                enabled = !busy && code.isNotBlank(),
                modifier = Modifier
                    .fillMaxWidth()
                    .height(48.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Primary),
            ) { Text("加入家庭组", fontWeight = FontWeight.SemiBold) }
        }

        Spacer(Modifier.height(18.dp))
        CardBox {
            Text("我收到的邀请（${pending.size}）", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Spacer(Modifier.height(4.dp))
            Text(
                "组长按你的用户名发出邀请后，会显示在这里。接受后才会加入家庭组。",
                fontSize = 11.5.sp,
                color = Sub,
            )
            Spacer(Modifier.height(8.dp))
            if (pending.isEmpty()) {
                Text(
                    "暂无待确认邀请",
                    fontSize = 12.5.sp,
                    color = Sub,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
            } else {
                pending.forEachIndexed { i, inv ->
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column(Modifier.weight(1f)) {
                            Text(
                                "「${inv.familyName}」",
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 13.5.sp,
                            )
                            Text(
                                "${inv.inviterNickname} 邀请你 · ${inv.createdDate} 发出",
                                fontSize = 11.sp,
                                color = Sub,
                            )
                        }
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        Button(
                            onClick = { vm.decideInvitation(inv.id, accept = true) },
                            enabled = !busy,
                            modifier = Modifier
                                .weight(1f)
                                .height(38.dp),
                            colors = ButtonDefaults.buttonColors(containerColor = Primary),
                        ) { Text("接受", fontSize = 13.sp) }
                        OutlinedButton(
                            onClick = { vm.decideInvitation(inv.id, accept = false) },
                            enabled = !busy,
                            modifier = Modifier
                                .weight(1f)
                                .height(38.dp),
                        ) { Text("拒绝", fontSize = 13.sp, color = Sub) }
                    }
                    if (i != pending.lastIndex) {
                        Spacer(Modifier.height(6.dp))
                    }
                }
            }
        }

        Spacer(Modifier.height(18.dp))
        TextButton(onClick = { vm.logout() }, modifier = Modifier.fillMaxWidth()) {
            Text("退出登录", color = Sub, fontSize = 13.sp)
        }
    }
}
