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
    /** 登录账号；后端 MemberBrief 一直返回，DTO 之前没接（家庭页要展示）。 */
    val username: String = "",
    val nickname: String,
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
    /** 登录账号，注册后**永不可改**；与可改的昵称是两回事。 */
    val username: String = "",
    val nickname: String = "",
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
    @SerialName("code_hidden") val codeHidden: Boolean = false,
)

@Serializable
data class StateView(
    val family: FamilyBrief = FamilyBrief(),
    val me: MemberBrief = MemberBrief(id = 0, nickname = ""),
    val members: List<MemberBrief> = emptyList(),
    @SerialName("unread_count") val unreadCount: Int = 0,
    // 单笔可赠送上限（= 可用积分），由后端给出，前端不自己拿 score 当上限
    @SerialName("transfer_max") val transferMax: Int = 0,
)

@Serializable
data class FamilyBrief(val id: Long = 0, val name: String = "")

@Serializable
data class ItemView(
    val id: Long,
    val name: String,
    val cost: Int,
    // 能不能换由后端算（余额可能已变），前端只展示
    val affordable: Boolean = false,
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
    val nickname: String = "",
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

/** 与后端 `repo.TrendPoint` 对齐：按天聚合，earned=当天正分之和，net=含负分的净变动。 */
@Serializable
data class TrendPoint(
    val date: String = "",
    val earned: Int = 0,
    val net: Int = 0,
)

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
    // 未按时睡觉的扣分口径（2026-11-05）：单位 30/60 分钟，每单位扣 N 分
    @SerialName("night_penalty_unit_min") val nightPenaltyUnitMin: Int = 30,
    @SerialName("night_penalty_points") val nightPenaltyPoints: Int = 0,
    val enabled: Boolean = true,
)

@Serializable
data class ScreenReportView(
    val id: Long = 0,
    @SerialName("report_date") val reportDate: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int = 0,
    // 判定结果由服务端给出（前端不自己算达标，见「规则全部由后端判断」）
    @SerialName("night_ok") val nightOk: Boolean? = null,
    val points: Int = 0,
    val source: String = "auto",
    val revision: Int = 1,
)

// ---------- 请求体 ----------

@Serializable
data class RegisterReq(
    val username: String,
    val password: String,
    val nickname: String,
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
    // 只发配置值；怎么扣由后端算
    @SerialName("night_penalty_unit_min") val nightPenaltyUnitMin: Int? = null,
    @SerialName("night_penalty_points") val nightPenaltyPoints: Int? = null,
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

/** 屏幕时间上报（2026-11-05 起走成员登录态，不再需要设备令牌）。 */
@Serializable
data class ScreenReportReq(
    // 设备标识：同一账号多设备时用于区分，服务端按「成员×日」汇总
    @SerialName("device_key") val deviceKey: String,
    @SerialName("device_model") val deviceModel: String,
    @SerialName("report_date") val reportDate: String,
    @SerialName("total_min") val totalMin: Int,
    // 必须可空：kotlinx 默认 encodeDefaults=false，Int 默认值 0 会被**直接省略**，
    // 服务端收到的是「没带 night_min」→ 当成未判定，夜间永远不加分。
    // 改成 Int? 后，传 0 与默认值 null 不同，才会真正编码进去。
    @SerialName("night_min") val nightMin: Int? = null,
    // 24 个小时桶（索引 = 小时，单位分钟）。2026-11-06 起上传，只用于**展示明细**，
    // 不参与判定或计分；但服务端在 24 个桶齐全时以桶之和为 total_min，
    // 保证「总时长」与「逐小时加起来」永远一致。
    val hourly: List<Int> = emptyList(),
)

@Serializable
data class ScreenReportResult(
    val changed: Boolean = false,
    @SerialName("report_date") val reportDate: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    val points: Int = 0,
    val revision: Int = 0,
)

/** 家庭屏幕详情：每人汇总 + 各设备明细。 */
@Serializable
data class FamilyScreenView(
    val date: String = "",
    val members: List<FamilyMemberDay> = emptyList(),
)

@Serializable
data class FamilyMemberDay(
    @SerialName("member_id") val memberId: Long = 0,
    val nickname: String = "",
    val role: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int? = null,
    @SerialName("night_ok") val nightOk: Boolean? = null,
    val points: Int = 0,
    val devices: List<DeviceDay> = emptyList(),
)

@Serializable
data class DeviceDay(
    @SerialName("device_key") val deviceKey: String = "",
    @SerialName("device_model") val deviceModel: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int? = null,
)

/** 某成员某天的屏幕时间详情（2026-11-06）。 */
@Serializable
data class MemberDetail(
    @SerialName("member_id") val memberId: Long = 0,
    val nickname: String = "",
    val role: String = "",
    val avatar: String = "",
    @SerialName("is_self") val isSelf: Boolean = false,
    val date: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int? = null,
    @SerialName("night_ok") val nightOk: Boolean? = null,
    val points: Int = 0,
    /** 当天是否已有结算行；false 表示还没结算，判定与积分一律显示「待结算」。 */
    val settled: Boolean = false,
    /** 24 桶，索引 = 小时，全部设备相加。 */
    val hourly: List<Int> = emptyList(),
    val devices: List<MemberDeviceDetail> = emptyList(),
    /** 近 7 天，含查询日，最旧在前。 */
    val days: List<DayPoint> = emptyList(),
)

@Serializable
data class MemberDeviceDetail(
    @SerialName("device_key") val deviceKey: String = "",
    @SerialName("device_model") val deviceModel: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int? = null,
    val hourly: List<Int> = emptyList(),
    /** 与 MemberDetail.days 同序的每日分钟数。 */
    val days: List<Int> = emptyList(),
)

@Serializable
data class DayPoint(
    val date: String = "",
    @SerialName("total_min") val totalMin: Int = 0,
    @SerialName("night_min") val nightMin: Int? = null,
)

/** 家庭码（2026-11-06）：8 位、唯一、永不可更改，可设为隐藏。 */
@Serializable
data class FamilyCodeView(
    val code: String = "",
    val hidden: Boolean = false,
)

/** 按家庭码搜索的结果。隐藏或不存在都是 found=false，无法区分。 */
@Serializable
data class FamilySearchView(
    val found: Boolean = false,
    @SerialName("family_id") val familyId: Long = 0,
    val name: String = "",
    @SerialName("member_count") val memberCount: Int = 0,
)

@Serializable
data class CreateFamilyReq(val name: String)

@Serializable
data class CodeHiddenReq(val hidden: Boolean)

@Serializable
data class FamilyCreatedView(
    @SerialName("family_id") val familyId: Long = 0,
    val name: String = "",
    val code: String = "",
)

@Serializable
data class RequestJoinReq(val code: String)

@Serializable
data class DecideRequestReq(
    @SerialName("invitation_id") val invitationId: Long,
    val accept: Boolean,
)

/** 个人资料更新（昵称 / 头像）。 */
@Serializable
data class ProfileReq(
    val nickname: String = "",
    val avatar: String = "",
)

@Serializable
data class MemberIdReq(@SerialName("member_id") val memberId: Long, val confirm: Boolean = false)

@Serializable
data class RenameReq(val nickname: String)

/**
 * 家庭组改名。
 *
 * ⚠️ 与 [RenameReq] 不是一个字段：后端 `/api/app/family/rename` 收 `name`，
 * `/api/app/member/rename` 收 `nickname`。之前这里复用了 RenameReq，
 * 提交的是 `{"nickname":...}` → 后端当成空名字，必然 400。
 * 因为一直没有入口调用，所以从没暴露过。
 */
@Serializable
data class FamilyRenameReq(val name: String)

// ---------- 待确认邀请（P3.1） ----------

@Serializable
data class InvitationView(
    val id: Long = 0,
    @SerialName("family_id") val familyId: Long = 0,
    @SerialName("inviter_id") val inviterId: Long = 0,
    @SerialName("invitee_id") val inviteeId: Long = 0,
    /** invite = 组长邀请我；request = 我凭家庭码申请加入 */
    val direction: String = "invite",
    val status: String = "",
    @SerialName("expires_at") val expiresAt: String = "",
    @SerialName("decided_at") val decidedAt: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("family_name") val familyName: String = "",
    @SerialName("inviter_nickname") val inviterNickname: String = "",
    @SerialName("invitee_nickname") val inviteeNickname: String = "",
) {
    val isPending: Boolean get() = status == "pending"
    val statusText: String get() = when (status) {
        "pending" -> "待确认"
        "accepted" -> "已接受"
        "rejected" -> "已拒绝"
        "cancelled" -> "已取消"
        "expired" -> "已过期"
        else -> status
    }
    // 服务端 time.Time 是 RFC3339，界面只展示日期部分。
    val createdDate: String get() = createdAt.take(10)
    val expireDate: String get() = expiresAt.take(10)
}

@Serializable
data class InviteByUsernameReq(val username: String)

@Serializable
data class InvitationIdReq(@SerialName("invitation_id") val invitationId: Long)

/** 列表接口返回 { items: [...] }，不能把 data 直接当成数组。 */
@Serializable
data class InvitationList(val items: List<InvitationView> = emptyList())

@Serializable
data class InvitationCancel(val cancelled: Boolean = false)

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


// ---------- 设备绑定与解绑（2026-11-07，docs 15.5） ----------

@Serializable
data class DeviceKeyReq(val device_key: String)

@Serializable
data class DecideUnbindReq(val request_id: Long, val approve: Boolean)

@Serializable
data class RequestIdReq(val request_id: Long)

/** 我绑定的设备（/api/app/screen/devices）。 */
@Serializable
data class MemberDeviceView(
    val id: Long = 0,
    val member_id: Long = 0,
    val device_key: String = "",
    val device_model: String = "",
    val first_seen_at: String = "",
    val last_seen_at: String = "",
)

@Serializable
data class MemberDeviceList(val devices: List<MemberDeviceView> = emptyList())

/** 一条解绑申请（自己的 / 组长待审批的）。 */
@Serializable
data class UnbindRequestView(
    val id: Long = 0,
    val member_id: Long = 0,
    val device_key: String = "",
    val device_model: String = "",
    val status: String = "pending",
    val created_at: String = "",
    val decided_at: String? = null,
    val member_nickname: String = "",
    val member_username: String = "",
)

@Serializable
data class UnbindRequestList(val items: List<UnbindRequestView> = emptyList())

@Serializable
data class UnbindResult(val unbound: Boolean = false, val device_key: String = "")

@Serializable
data class CancelResult(val cancelled: Boolean = false)

@Serializable
data class CancelAccountReq(val password: String)

@Serializable
data class CancelAccountResult(val cancelled: Boolean = false)
