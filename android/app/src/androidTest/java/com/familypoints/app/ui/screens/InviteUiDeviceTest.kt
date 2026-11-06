package com.familypoints.app.ui.screens

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.navigation.compose.rememberNavController
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.ApiService
import com.familypoints.app.data.api.Envelope
import com.familypoints.app.data.api.InvitationCancel
import com.familypoints.app.data.api.InvitationIdReq
import com.familypoints.app.data.api.InvitationList
import com.familypoints.app.data.api.InvitationView
import com.familypoints.app.data.api.InviteByUsernameReq
import com.familypoints.app.data.api.InviteCodeView
import com.familypoints.app.data.api.MeView
import com.familypoints.app.data.local.TokenStore
import com.familypoints.app.screen.ReportScheduler
import com.familypoints.app.ui.AppViewModel
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.mockito.ArgumentMatchers.any
import org.mockito.Mockito.mock
import org.mockito.Mockito.`when`

/**
 * P3.1 新界面的真机界面级验证。
 *
 * 目的：在真实设备上渲染「组长按用户名邀请」与「未入组用户收到邀请」两个 composable，
 * 断言关键文案/控件存在，并模拟输入与点击，证明界面代码在真机上不崩、元素齐全。
 *
 * 不依赖后端：用 Mockito 假 ApiClient 喂样例数据。
 * 服务端契约（状态机、审计、通知）已由 server/scripts/p12_invite_check.sh 覆盖。
 */
@RunWith(AndroidJUnit4::class)
class InviteUiDeviceTest {

    @get:Rule
    val rule = createComposeRule()

    private fun newVm(): AppViewModel {
        val api = mock(ApiService::class.java)

        `when`(api.inviteCode()).thenReturn(
            Envelope(ok = true, data = InviteCodeView(inviteCode = "ABCDEF12"))
        )
        `when`(api.familyInvitations()).thenReturn(
            Envelope(
                ok = true,
                data = InvitationList(
                    listOf(
                        InvitationView(
                            id = 1,
                            status = "pending",
                            inviteeNickname = "小明",
                            createdAt = "2026-11-06T10:00:00Z",
                            expiresAt = "2026-11-13T10:00:00Z",
                        )
                    )
                ),
            )
        )
        `when`(api.myInvitations()).thenReturn(
            Envelope(
                ok = true,
                data = InvitationList(
                    listOf(
                        InvitationView(
                            id = 2,
                            status = "pending",
                            familyName = "幸福一家",
                            inviterNickname = "爸爸",
                            createdAt = "2026-11-06T10:00:00Z",
                        )
                    )
                ),
            )
        )
        `when`(api.inviteByUsername(any(InviteByUsernameReq::class.java)))
            .thenReturn(Envelope(ok = true, data = InvitationView(id = 99, status = "pending")))
        `when`(api.cancelInvitation(any(InvitationIdReq::class.java)))
            .thenReturn(Envelope(ok = true, data = InvitationCancel(cancelled = true)))
        `when`(api.acceptInvitation(any(InvitationIdReq::class.java)))
            .thenReturn(Envelope(ok = true, data = InvitationView(id = 2, status = "accepted")))
        `when`(api.rejectInvitation(any(InvitationIdReq::class.java)))
            .thenReturn(Envelope(ok = true, data = InvitationView(id = 2, status = "rejected")))

        val client = mock(ApiClient::class.java)
        `when`(client.service).thenReturn(api)

        return AppViewModel(
            client,
            mock(TokenStore::class.java),
            mock(ReportScheduler::class.java),
        )
    }

    @Test
    fun leaderInviteCardRendersAndSends() {
        val vm = newVm()
        rule.setContent {
            MaterialTheme {
                ManageMembersScreen(vm = vm, nav = rememberNavController())
            }
        }
        rule.waitForIdle()

        rule.onNodeWithText("按用户名邀请").assertIsDisplayed()
        rule.onNodeWithText("本组已发邀请（1）").assertIsDisplayed()
        rule.onNodeWithText("小明").assertIsDisplayed()
        rule.onNodeWithText("取消").assertIsDisplayed()
        rule.onNodeWithText("邀请码").assertIsDisplayed()

        // 输入被邀请人用户名并点发送，验证交互路径不崩
        rule.onNodeWithText("对方的用户名").performTextInput("xiaoming")
        rule.onNodeWithText("发送邀请").performClick()
        rule.waitForIdle()
    }

    @Test
    fun receivedInviteCardRendersAndAccepts() {
        val vm = newVm()
        rule.setContent {
            MaterialTheme {
                JoinFamilyScreen(vm = vm, me = MeView(id = 10, nickname = "新成员"))
            }
        }
        rule.waitForIdle()

        rule.onNodeWithText("我收到的邀请（1）").assertIsDisplayed()
        rule.onNodeWithText("「幸福一家」").assertIsDisplayed()
        rule.onNodeWithText("接受").assertIsDisplayed()
        rule.onNodeWithText("拒绝").assertIsDisplayed()

        rule.onNodeWithText("接受").performClick()
        rule.waitForIdle()
    }
}
