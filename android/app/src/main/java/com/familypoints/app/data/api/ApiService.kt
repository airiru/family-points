package com.familypoints.app.data.api

import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Header
import retrofit2.http.POST
import retrofit2.http.Query

/**
 * 后端接口。前缀与鉴权严格对应：
 * - /api/app/...    需 Bearer access token（由拦截器统一注入）
 * - /api/device/... 需 X-Device-Token，不走 JWT
 */
interface ApiService {

    // ---------- 公开 ----------
    @POST("api/register")
    suspend fun register(@Body body: RegisterReq): Envelope<AuthResult>

    @POST("api/app/login")
    suspend fun login(@Body body: LoginReq): Envelope<AuthResult>

    @POST("api/app/refresh")
    suspend fun refresh(@Body body: RefreshReq): Envelope<AuthResult>

    // ---------- 账号 ----------
    @GET("api/app/me")
    suspend fun me(): Envelope<MeView>

    @POST("api/app/password")
    suspend fun changePassword(@Body body: PasswordReq): Envelope<Unit>

    // ---------- 家庭组 ----------
    @POST("api/app/family/join")
    suspend fun join(@Body body: JoinReq): Envelope<Unit>

    @GET("api/app/family")
    suspend fun family(): Envelope<FamilyView>

    @GET("api/app/family/members")
    suspend fun members(): Envelope<MemberList>

    @POST("api/app/family/rename")
    suspend fun renameFamily(@Body body: RenameReq): Envelope<Unit>

    @GET("api/app/family/invite-code")
    suspend fun inviteCode(): Envelope<InviteCodeView>

    @POST("api/app/family/invite-code")
    suspend fun resetInviteCode(): Envelope<InviteCodeView>

    @POST("api/app/family/leave")
    suspend fun leaveFamily(@Body body: ConfirmReq): Envelope<Unit>

    @POST("api/app/family/transfer-leader")
    suspend fun transferLeader(@Body body: TransferLeaderReq): Envelope<Unit>

    // ---------- 成员管理（组长） ----------
    @POST("api/app/member/remove")
    suspend fun kickMember(@Body body: MemberIdReq): Envelope<Unit>

    @POST("api/app/member/hidden")
    suspend fun setHidden(@Body body: HiddenReq): Envelope<Unit>

    @POST("api/app/member/rename")
    suspend fun renameMember(@Body body: RenameMemberReq): Envelope<Unit>

    // ---------- 概览 / 记录 / 榜单 ----------
    @GET("api/app/state")
    suspend fun state(): Envelope<StateView>

    @GET("api/app/records")
    suspend fun records(
        @Query("member_id") memberId: Long? = null,
        @Query("keyword") keyword: String? = null,
        @Query("source") source: String? = null,
        @Query("limit") limit: Int = 50,
        @Query("offset") offset: Int = 0,
    ): Envelope<RecordPage>

    @GET("api/app/stats/ranking")
    suspend fun ranking(
        @Query("type") type: String = "points",
        @Query("period") period: String = "week",
    ): Envelope<RankingView>

    @GET("api/app/stats/trend")
    suspend fun trend(
        @Query("member_id") memberId: Long? = null,
        @Query("period") period: String = "week",
    ): Envelope<TrendView>

    // ---------- 商城 ----------
    @GET("api/app/item/list")
    suspend fun items(): Envelope<ItemList>

    @POST("api/app/item/redeem")
    suspend fun redeem(@Body body: RedeemReq): Envelope<Unit>

    @POST("api/app/item/save")
    suspend fun saveItem(@Body body: ItemSaveReq): Envelope<ItemView>

    @POST("api/app/item/delete")
    suspend fun deleteItem(@Body body: ItemIdReq): Envelope<Unit>

    // ---------- 商品券（兑换后核销，docs 9.6） ----------
    @GET("api/app/coupons")
    suspend fun coupons(): Envelope<CouponList>

    @GET("api/app/coupons/pending")
    suspend fun pendingCoupons(): Envelope<CouponList>

    @GET("api/app/coupon/detail")
    suspend fun couponDetail(@Query("coupon_id") couponId: Long): Envelope<CouponDetailView>

    @POST("api/app/coupon/use")
    suspend fun useCoupon(@Body body: CouponIdReq): Envelope<CouponView>

    @POST("api/app/coupon/vote")
    suspend fun voteCoupon(@Body body: CouponVoteReq): Envelope<CouponVoteResult>

    // ---------- 赠送 ----------
    @POST("api/app/transfer/send")
    suspend fun sendTransfer(@Body body: TransferReq): Envelope<Unit>

    @GET("api/app/transfer/list")
    suspend fun transfers(@Query("limit") limit: Int = 50): Envelope<RecordPage>

    // ---------- 任务 ----------
    @POST("api/app/task/create")
    suspend fun createTask(@Body body: CreateTaskReq): Envelope<TaskView>

    @GET("api/app/task/list")
    suspend fun tasks(@Query("status") status: String? = null): Envelope<TaskList>

    @GET("api/app/task/detail")
    suspend fun taskDetail(@Query("task_id") taskId: Long): Envelope<TaskDetailView>

    @POST("api/app/task/claim")
    suspend fun claimTask(@Body body: TaskIdReq): Envelope<TaskView>

    @POST("api/app/task/abandon")
    suspend fun abandonTask(@Body body: TaskIdReq): Envelope<Unit>

    @POST("api/app/task/close")
    suspend fun closeTask(@Body body: TaskIdReq): Envelope<TaskView>

    @POST("api/app/task/submit")
    suspend fun submitTask(@Body body: SubmitReq): Envelope<SubmissionView>

    @POST("api/app/task/vote")
    suspend fun voteTask(@Body body: VoteReq): Envelope<VoteResult>

    // ---------- 通知 ----------
    @GET("api/app/notification/list")
    suspend fun notifications(
        @Query("unread") unread: Int? = null,
        @Query("limit") limit: Int = 50,
    ): Envelope<NotificationList>

    @POST("api/app/notification/read")
    suspend fun readNotification(@Body body: NotificationReadReq): Envelope<Unit>

    // ---------- 睡觉规则与屏幕时间 ----------
    @GET("api/app/rule")
    suspend fun rules(): Envelope<RuleList>

    @POST("api/app/rule/save")
    suspend fun saveRule(@Body body: RuleSaveReq): Envelope<RuleView>

    @GET("api/app/screen-rule")
    suspend fun screenRules(): Envelope<ScreenRuleList>

    @POST("api/app/screen-rule/save")
    suspend fun saveScreenRule(@Body body: ScreenRuleSaveReq): Envelope<ScreenRuleView>

    @GET("api/app/screen/reports")
    suspend fun screenReports(
        @Query("member_id") memberId: Long? = null,
        @Query("limit") limit: Int = 30,
    ): Envelope<ScreenReportList>

    // ---------- 设备上报（独立鉴权，不走拦截器） ----------
    @POST("api/device/screen/report")
    suspend fun deviceReport(
        @Header("X-Device-Token") token: String,
        @Body body: DeviceReportReq,
    ): Envelope<Unit>
}

@kotlinx.serialization.Serializable
data class PasswordReq(val password: String)

@kotlinx.serialization.Serializable
data class ConfirmReq(val confirm: Boolean = true)

@kotlinx.serialization.Serializable
data class RenameMemberReq(@kotlinx.serialization.SerialName("member_id") val memberId: Long, val name: String)

@kotlinx.serialization.Serializable
data class InviteCodeView(
    @kotlinx.serialization.SerialName("invite_code") val inviteCode: String = "",
    @kotlinx.serialization.SerialName("invite_code_expires_at") val expiresAt: String? = null,
)

@kotlinx.serialization.Serializable
data class MemberList(val items: List<MemberBrief> = emptyList())

@kotlinx.serialization.Serializable
data class ItemList(val items: List<ItemView> = emptyList())

@kotlinx.serialization.Serializable
data class RuleList(val items: List<RuleView> = emptyList())

@kotlinx.serialization.Serializable
data class ScreenRuleList(
    // 后端返回的是 screen_rules，不是 items —— 写错的话 decode 成空列表，
    // 上报 Worker 拿不到夜间配置，night_min 永远是 null（且被 runCatching 吞掉不报错）
    @kotlinx.serialization.SerialName("screen_rules") val items: List<ScreenRuleView> = emptyList(),
)

@kotlinx.serialization.Serializable
data class ScreenReportList(
    @kotlinx.serialization.SerialName("reports") val items: List<ScreenReportView> = emptyList(),
)
