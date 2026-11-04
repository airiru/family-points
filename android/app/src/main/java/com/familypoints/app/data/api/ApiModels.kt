package com.familypoints.app.data.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** 全站统一响应：{ ok, data, error } */
@Serializable
data class Envelope<T>(
    val ok: Boolean = false,
    val data: T? = null,
    val error: String? = null,
)

@Serializable
data class TokenPair(
    @SerialName("access_token") val accessToken: String,
    @SerialName("refresh_token") val refreshToken: String,
)

@Serializable
data class MemberBrief(
    val id: Long,
    val name: String,
    val role: String = "member",
    val avatar: String? = null,
    val score: Int = 0,
    @SerialName("frozen_score") val frozenScore: Int = 0,
    val hidden: Boolean = false,
) {
    val isLeader: Boolean get() = role == "leader"
    val totalScore: Int get() = score + frozenScore
}

@Serializable
data class MeView(
    val id: Long,
    val name: String = "",
    val role: String = "member",
    val score: Int = 0,
    @SerialName("frozen_score") val frozenScore: Int = 0,
    val avatar: String? = null,
    @SerialName("family_id") val familyId: Long? = null,
) {
    val isLeader: Boolean get() = role == "leader"
}

@Serializable
data class FamilyView(
    val id: Long,
    val name: String,
    @SerialName("invite_code") val inviteCode: String? = null,
    @SerialName("invite_code_expires_at") val inviteCodeExpiresAt: String? = null,
)

@Serializable
data class StateView(
    val family: FamilyBrief = FamilyBrief(),
    val me: MemberBrief = MemberBrief(0, ""),
    val members: List<MemberBrief> = emptyList(),
    @SerialName("unread_count") val unreadCount: Int = 0,
)

@Serializable
data class FamilyBrief(val id: Long = 0, val name: String = "")

@Serializable
data class ItemView(
    val id: Long,
    val name: String,
    val cost: Int,
)

@Serializable
data class RecordView(
    val id: Long,
    @SerialName("member_id") val memberId: Long,
    @SerialName("member_name") val memberName: String = "",
    val title: String = "",
    val points: Int = 0,
    @SerialName("occurred_at") val occurredAt: String = "",
    @SerialName("added_at") val addedAt: String = "",
    val source: String = "manual",
)

@Serializable
data class RecordPage(
    val items: List<RecordView> = emptyList(),
    val total: Int = 0,
    val limit: Int = 50,
    val offset: Int = 0,
)

@Serializable
data class RankingRow(
    @SerialName("member_id") val memberId: Long,
    val name: String = "",
    val avatar: String? = null,
    val score: Int = 0,
    @SerialName("frozen_score") val frozenScore: Int = 0,
    @SerialName("period_score") val periodScore: Int = 0,
    val streak: Int = 0,
)

@Serializable
data class RankingView(
    val type: String = "points",
    val period: String = "week",
    val rows: List<RankingRow> = emptyList(),
)

@Serializable
data class TrendView(
    val period: String = "week",
    val points: List<TrendPoint> = emptyList(),
)

@Serializable
data class TrendPoint(val label: String = "", val value: Int = 0)

@Serializable
data class TaskView(
    val id: Long,
    val title: String = "",
    val points: Int = 0,
    @SerialName("publisher_id") val publisherId: Long = 0,
    @SerialName("escrow_points") val escrowPoints: Int = 0,
    @SerialName("assignee_id") val assigneeId: Long? = null,
    @SerialName("completed_by") val completedBy: Long? = null,
    @SerialName("due_at") val dueAt: String? = null,
    val status: String = "open",
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class TaskList(val tasks: List<TaskView> = emptyList())

@Serializable
data class SubmissionView(
    val id: Long,
    @SerialName("task_id") val taskId: Long = 0,
    @SerialName("member_id") val memberId: Long = 0,
    val note: String = "",
    val status: String = "submitted",
    @SerialName("vote_deadline") val voteDeadline: String? = null,
    @SerialName("created_at") val createdAt: String = "",
)

/** 任务详情：task + 全部提交 + 每个提交的票数与汇总。 */
@Serializable
data class TaskDetailView(
    val task: TaskView = TaskView(0),
    val submissions: List<SubmissionView> = emptyList(),
    val votes: Map<String, List<TaskVoteItem>> = emptyMap(),
    val tally: Map<String, VoteTally> = emptyMap(),
    val voters: Int = 0,
)

@Serializable
data class TaskVoteItem(
    val id: Long = 0,
    @SerialName("submission_id") val submissionId: Long = 0,
    @SerialName("voter_id") val voterId: Long = 0,
    val vote: String = "approve",
)

@Serializable
data class VoteTally(
    val approve: Int = 0,
    val reject: Int = 0,
    val need: Int = 0,
)

@Serializable
data class VoteResult(
    val status: String = "submitted",
    val approve: Int = 0,
    val reject: Int = 0,
    val voters: Int = 0,
    val transferred: Boolean = false,
)

@Serializable
data class NotificationView(
    val id: Long,
    val type: String = "",
    val title: String = "",
    val body: String? = null,
    @SerialName("read_at") val readAt: String? = null,
    @SerialName("created_at") val createdAt: String = "",
) {
    val unread: Boolean get() = readAt.isNullOrBlank()
}

@Serializable
data class NotificationList(val items: List<NotificationView> = emptyList())

@Serializable
data class RuleView(
    val id: Long = 0,
    val name: String = "",
    val points: Int = 0,
    @SerialName("trigger_type") val triggerType: String = "screen_night",
    val streaks: List<StreakTier> = emptyList(),
)

@Serializable
data class StreakTier(
    val every: Int = 0,
    val bonus: Int = 0,
    val mode: String = "repeat",
)

@Serializable
data class ScreenRuleView(
    @SerialName("member_id") val memberId: Long = 0,
    @SerialName("threshold_min") val thresholdMin: Int = 0,
    @SerialName("bonus_points") val bonusPoints: Int = 0,
    @SerialName("penalty_points") val penaltyPoints: Int = 0,
    @SerialName("night_start_hour") val nightStartHour: Int? = null,
    @SerialName("night_end_hour") val nightEndHour: Int? = null,
    @SerialName("night_max_min") val nightMaxMin: Int? = null,
    val enabled: Boolean = true,
)

@Serializable
data class ScreenReportView(
    val id: Long = 0,
    @SerialName("report_date") val reportDate: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int = 0,
    val points: Int = 0,
    val source: String = "auto",
    val revision: Int = 1,
)

// ---------- 请求体 ----------

@Serializable
data class RegisterReq(
    val username: String,
    val password: String,
    val name: String,
    @SerialName("create_family") val createFamily: Boolean = false,
    @SerialName("family_name") val familyName: String = "",
)

@Serializable
data class LoginReq(val username: String, val password: String)

@Serializable
data class RefreshReq(@SerialName("refresh_token") val refreshToken: String)

@Serializable
data class AuthResult(
    val member: MeView? = null,
    val token: TokenPair? = null,
)

@Serializable
data class JoinReq(@SerialName("invite_code") val inviteCode: String)

@Serializable
data class RedeemReq(@SerialName("item_id") val itemId: Long)

@Serializable
data class TransferReq(
    @SerialName("to_member_id") val toMemberId: Long,
    val amount: Int,
    val note: String = "",
)

@Serializable
data class CreateTaskReq(val title: String, val points: Int, @SerialName("due_at") val dueAt: String? = null)

@Serializable
data class TaskIdReq(@SerialName("task_id") val taskId: Long)

@Serializable
data class SubmitReq(@SerialName("task_id") val taskId: Long, val note: String)

@Serializable
data class VoteReq(@SerialName("submission_id") val submissionId: Long, val vote: String)

@Serializable
data class ItemSaveReq(
    val id: Long? = null,
    val name: String,
    val cost: Int,
)

@Serializable
data class ItemIdReq(@SerialName("item_id") val itemId: Long)

@Serializable
data class ScreenRuleSaveReq(
    @SerialName("member_id") val memberId: Long,
    @SerialName("threshold_min") val thresholdMin: Int,
    @SerialName("bonus_points") val bonusPoints: Int,
    @SerialName("penalty_points") val penaltyPoints: Int,
    @SerialName("night_start_hour") val nightStartHour: Int,
    @SerialName("night_end_hour") val nightEndHour: Int,
    @SerialName("night_max_min") val nightMaxMin: Int,
    val enabled: Boolean = true,
)

@Serializable
data class RuleSaveReq(
    val id: Long? = null,
    val name: String,
    val points: Int,
    @SerialName("trigger_type") val triggerType: String = "screen_night",
    val streaks: List<StreakTier> = emptyList(),
)

/** 设备上报（X-Device-Token 鉴权，无 JWT）。 */
@Serializable
data class DeviceReportReq(
    @SerialName("report_date") val reportDate: String,
    @SerialName("total_min") val totalMin: Int,
    // 必须可空：kotlinx 默认 encodeDefaults=false，Int 默认值 0 会被**直接省略**，
    // 服务端收到的是「没带 night_min」→ 当成未判定，夜间永远不加分。
    // 改成 Int? 后，传 0 与默认值 null 不同，才会真正编码进去。
    @SerialName("night_min") val nightMin: Int? = null,
)

@Serializable
data class MemberIdReq(@SerialName("member_id") val memberId: Long, val confirm: Boolean = false)

@Serializable
data class RenameReq(val name: String)

@Serializable
data class HiddenReq(@SerialName("member_id") val memberId: Long, val hidden: Boolean)

@Serializable
data class TransferLeaderReq(@SerialName("member_id") val memberId: Long)

@Serializable
data class NotificationReadReq(val id: Long = 0, val all: Boolean = false)

// ---------- 商品券（docs 9.6） ----------

/** 券状态：unused 未使用 / pending 待核销 / redeemed 已核销 */
@Serializable
data class CouponView(
    val id: Long,
    @SerialName("member_id") val memberId: Long = 0,
    @SerialName("redemption_id") val redemptionId: Long = 0,
    @SerialName("item_name") val itemName: String = "",
    val cost: Int = 0,
    val status: String = "unused",
    @SerialName("reject_count") val rejectCount: Int = 0,
    @SerialName("vote_deadline") val voteDeadline: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("settled_at") val settledAt: String? = null,
)

@Serializable
data class CouponList(val items: List<CouponView> = emptyList())

@Serializable
data class CouponVoteItem(
    val id: Long = 0,
    @SerialName("coupon_id") val couponId: Long = 0,
    @SerialName("voter_id") val voterId: Long = 0,
    val vote: String = "approve",
)

@Serializable
data class CouponDetailView(
    val coupon: CouponView = CouponView(0),
    val votes: List<CouponVoteItem> = emptyList(),
)

@Serializable
data class CouponVoteResult(
    val status: String = "pending",
    val approve: Int = 0,
    val reject: Int = 0,
    val voters: Int = 0,
)

@Serializable
data class CouponIdReq(@SerialName("coupon_id") val couponId: Long)

@Serializable
data class CouponVoteReq(
    @SerialName("coupon_id") val couponId: Long,
    val vote: String,
)
