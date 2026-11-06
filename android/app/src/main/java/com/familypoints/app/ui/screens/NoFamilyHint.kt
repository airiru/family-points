package com.familypoints.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.NavHostController
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.Sub

/**
 * 「还没加入家庭」时各 tab 的占位。
 *
 * 2026-11-06 反馈：未加入家庭不能只显示一个页面——底部导航必须还在，
 * 用户要能切到「我的」去看家庭状态。所以这里只替换**内容区**，
 * 给出说明和一个直达「所在家庭」的按钮。
 */
@Composable
fun NoFamilyHint(nav: NavHostController, title: String, need: String) {
    Column(Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().padding(16.dp),
            verticalArrangement = Arrangement.Top,
        ) {
            CardBox {
                Text("还没有加入家庭", fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                Text(
                    "$need 需要先加入一个家庭。用家庭码申请加入（需组长审批），或者自己创建一个家。",
                    fontSize = 12.sp, color = Sub, lineHeight = 18.sp,
                    modifier = Modifier.padding(top = 6.dp),
                )
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = { nav.navigate(com.familypoints.app.ui.nav.Routes.FAMILY) },
                    modifier = Modifier.fillMaxWidth().height(44.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = Primary),
                ) { Text("去加入家庭") }
            }
            Spacer(Modifier.height(10.dp))
            Text(
                "也可以在底部「我的」里随时查看家庭状态。",
                fontSize = 11.5.sp, color = Sub,
                modifier = Modifier.fillMaxWidth(),
                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
            )
        }
    }
}
