package com.familypoints.app.ui

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.CancelAccountReq
import com.familypoints.app.data.api.CancelResult
import com.familypoints.app.data.api.DecideUnbindReq
import com.familypoints.app.data.api.DeviceKeyReq
import com.familypoints.app.data.api.FeedbackReq
import com.familypoints.app.data.api.MemberDeviceView
import com.familypoints.app.data.api.RequestIdReq
import com.familypoints.app.data.api.UnbindRequestView
import com.familypoints.app.data.api.CouponIdReq
import com.familypoints.app.data.api.CouponView
import com.familypoints.app.data.api.CouponVoteReq
import com.familypoints.app.data.api.CreateTaskReq
import com.familypoints.app.data.api.FamilyRenameReq
import com.familypoints.app.data.api.CodeHiddenReq
import com.familypoints.app.data.api.FamilySearchView
import com.familypoints.app.data.api.CreateFamilyReq
import com.familypoints.app.data.api.RequestJoinReq
import com.familypoints.app.data.api.DecideRequestReq
import com.familypoints.app.data.api.FamilyScreenView
import com.familypoints.app.data.api.HiddenReq
import com.familypoints.app.data.api.ItemIdReq
import com.familypoints.app.data.api.ItemSaveReq
import com.familypoints.app.data.api.InvitationIdReq
import com.familypoints.app.data.api.InvitationView
import com.familypoints.app.data.api.InviteByUsernameReq
import com.familypoints.app.data.api.ItemView
import com.familypoints.app.data.api.JoinReq
import com.familypoints.app.data.api.LoginReq
import com.familypoints.app.data.api.MeView
import com.familypoints.app.data.api.MemberDetail
import com.familypoints.app.data.api.MemberBrief
import com.familypoints.app.data.api.MemberIdReq
import com.familypoints.app.data.api.NotificationReadReq
import com.familypoints.app.data.api.NotificationView
import com.familypoints.app.data.api.PasswordReq
import com.familypoints.app.data.api.ProfileReq
import com.familypoints.app.data.api.RankingRow
import com.familypoints.app.data.api.RecordView
import com.familypoints.app.data.api.RedeemReq
import com.familypoints.app.data.api.ScreenReportReq
import com.familypoints.app.data.api.RegisterReq
import com.familypoints.app.data.api.RenameMemberReq
import com.familypoints.app.data.api.RuleSaveReq
import com.familypoints.app.data.api.RuleView
import com.familypoints.app.data.api.ScreenReportView
import com.familypoints.app.data.api.ScreenRuleSaveReq
import com.familypoints.app.data.api.ScreenRuleView
import com.familypoints.app.data.api.StreakTier
import com.familypoints.app.data.api.SubmitReq
import com.familypoints.app.data.api.TaskDetailView
import com.familypoints.app.data.api.TaskIdReq
import com.familypoints.app.data.api.TaskView
import com.familypoints.app.data.api.TrendPoint
import com.familypoints.app.data.api.TransferLeaderReq
import com.familypoints.app.data.api.TransferReq
import com.familypoints.app.data.api.VoteReq
import com.familypoints.app.data.local.TokenStore
import com.familypoints.app.screen.ReportScheduler
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.Job
import kotlinx.coroutines.isActive
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import javax.inject.Inject

/** 登录态：决定进入登录页还是主界面。 */
sealed interface Session {
    data object Loading : Session
    data object LoggedOut : Session
    /** 已登录但还没加入家庭组。 */
    data class NoFamily(val me: MeView) : Session
    data class Ready(val me: MeView) : Session
}

private const val TAG = "AppViewModel"

@HiltViewModel
class AppViewModel @Inject constructor(
    private val apiClient: ApiClient,
    private val tokenStore: TokenStore,
    private val scheduler: ReportScheduler,
) : ViewModel() {

    private val api get() = apiClient.service

    private val _session = MutableStateFlow<Session>(Session.Loading)
    val session: StateFlow<Session> = _session.asStateFlow()

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    private val _familyName = MutableStateFlow("")
    val familyName: StateFlow<String> = _familyName.asStateFlow()

    private val _members = MutableStateFlow<List<MemberBrief>>(emptyList())
    val members: StateFlow<List<MemberBrief>> = _members.asStateFlow()

    private val _unread = MutableStateFlow(0)
    val unread: StateFlow<Int> = _unread.asStateFlow()

    private val _ranking = MutableStateFlow<List<RankingRow>>(emptyList())
    val ranking: StateFlow<List<RankingRow>> = _ranking.asStateFlow()

    /** 积分趋势（全组，按天）。period 只作展示参数，判定与聚合都在后端。 */
    private val _trend = MutableStateFlow<List<TrendPoint>>(emptyList())
    val trend: StateFlow<List<TrendPoint>> = _trend.asStateFlow()

    private val _records = MutableStateFlow<List<RecordView>>(emptyList())
    val records: StateFlow<List<RecordView>> = _records.asStateFlow()

    private val _items = MutableStateFlow<List<ItemView>>(emptyList())
    val items: StateFlow<List<ItemView>> = _items.asStateFlow()

    /** 单笔可赠送上限，由后端 state 接口给出。 */
    private val _transferMax = MutableStateFlow(0)
    val transferMax: StateFlow<Int> = _transferMax.asStateFlow()

    private val _tasks = MutableStateFlow<List<TaskView>>(emptyList())
    val tasks: StateFlow<List<TaskView>> = _tasks.asStateFlow()

    private val _taskDetail = MutableStateFlow<TaskDetailView?>(null)
    val taskDetail: StateFlow<TaskDetailView?> = _taskDetail.asStateFlow()

    private val _notifications = MutableStateFlow<List<NotificationView>>(emptyList())
    val notifications: StateFlow<List<NotificationView>> = _notifications.asStateFlow()

    private val _rules = MutableStateFlow<List<RuleView>>(emptyList())
    val rules: StateFlow<List<RuleView>> = _rules.asStateFlow()

    private val _screenRules = MutableStateFlow<List<ScreenRuleView>>(emptyList())
    val screenRules: StateFlow<List<ScreenRuleView>> = _screenRules.asStateFlow()

    private val _coupons = MutableStateFlow<List<CouponView>>(emptyList())
    val coupons: StateFlow<List<CouponView>> = _coupons.asStateFlow()

    /** 本组「待我核销」的券（别人发起、等我投票）。 */
    private val _pendingCoupons = MutableStateFlow<List<CouponView>>(emptyList())
    val pendingCoupons: StateFlow<List<CouponView>> = _pendingCoupons.asStateFlow()

    private val _inviteCode = MutableStateFlow<String?>(null)
    val inviteCode: StateFlow<String?> = _inviteCode.asStateFlow()

    /** 本机成员的屏幕时间结算历史（服务端结果，含 night_ok 判定）。 */
    private val _screenReports = MutableStateFlow<List<ScreenReportView>>(emptyList())
    val screenReports: StateFlow<List<ScreenReportView>> = _screenReports.asStateFlow()

    init {
        restoreSession()
        startUnreadPolling()
        // 会话失效（停用/注销/令牌吊销）：清本地状态回登录页（2026-11-07 用户反馈）
        viewModelScope.launch {
            apiClient.sessionInvalid.collect {
                tokenStore.clear()
                clearScreenDetail()
                _members.value = emptyList()
                _ranking.value = emptyList()
                _notifications.value = emptyList()
                _session.value = Session.LoggedOut
                _message.value = "登录已失效，请重新登录"
            }
        }
    }

    // ---------------- 会话 ----------------

    private fun restoreSession() {
        viewModelScope.launch {
            val token = tokenStore.accessTokenOnce()
            if (token.isNullOrBlank()) {
                clearScreenDetail()
                _session.value = Session.LoggedOut
                return@launch
            }
            // 用 try/catch 而不是 runCatching{}.onSuccess{}：
            // onSuccess/onFailure 是 inline，理论上能隐式继承 suspend 上下文，
            // 但这里要在失败分支里读 DataStore（suspend），显式 try/catch 语义更清楚、不依赖内联细节。
            try {
                val me = api.me().data
                if (me != null) {
                    _session.value = if (me.familyId == null) Session.NoFamily(me) else Session.Ready(me)
                    if (me.familyId != null) {
                        loadOverview()
                        scheduler.runNow() // 打开 App 时补算一次
                    }
                } else {
                    _session.value = Session.LoggedOut
                }
            } catch (e: Exception) {
                // 网络失败不踢登录态：保留本地身份，后续请求的 401 会触发刷新或登出
                val id = tokenStore.memberIdOnce() ?: 0L
                val familyId = tokenStore.familyId.first()
                _session.value = Session.Ready(
                    MeView(id = id, nickname = "", familyId = familyId)
                )
            }
        }
    }

    fun login(username: String, password: String) = launchBusy {
        val resp = api.login(LoginReq(username.trim(), password))
        handleAuth(resp.data?.member, resp.data?.token?.accessToken, resp.data?.token?.refreshToken, resp.error)
    }

    fun register(username: String, password: String, nickname: String, createFamily: Boolean, familyName: String) =
        launchBusy {
            val resp = api.register(
                RegisterReq(
                    username = username.trim(),
                    password = password,
                    nickname = nickname.trim().ifBlank { username.trim() },
                    createFamily = createFamily,
                    familyName = familyName.trim(),
                )
            )
            handleAuth(resp.data?.member, resp.data?.token?.accessToken, resp.data?.token?.refreshToken, resp.error)
        }

    private suspend fun handleAuth(me: MeView?, access: String?, refresh: String?, error: String?) {
        if (me == null || access == null || refresh == null) {
            _message.value = error ?: "登录失败"
            return
        }
        tokenStore.saveSession(access, refresh, me.id, me.nickname, me.role, me.familyId)
        _session.value = if (me.familyId == null) Session.NoFamily(me) else Session.Ready(me)
        if (me.familyId != null) {
            loadOverview()
            scheduler.runNow()
        }
    }

    fun joinFamily(code: String) = launchBusy {
        val resp = api.join(JoinReq(code.trim()))
        if (!resp.ok) {
            _message.value = resp.error ?: "加入失败"
            return@launchBusy
        }
        val me = api.me().data
        if (me != null) {
            tokenStore.saveSession(
                tokenStore.accessTokenOnce().orEmpty(),
                tokenStore.refreshTokenOnce().orEmpty(),
                me.id, me.nickname, me.role, me.familyId,
            )
            _session.value = Session.Ready(me)
            loadOverview()
            scheduler.runNow()
        }
    }

    /** 提交用户反馈（设置页）。 */
    fun submitFeedback(content: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.submitFeedback(FeedbackReq(content))
        if (resp.ok) {
            _message.value = "已提交，感谢反馈"
            onDone()
        } else {
            _message.value = resp.error ?: "提交失败"
        }
    }

    /** 注销账号：成功即本地登出（服务端已删账号与全部会话）。 */
    fun cancelAccount(password: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.cancelAccount(CancelAccountReq(password))
        if (resp.ok) {
            tokenStore.clear()
            clearScreenDetail()
            _members.value = emptyList()
            _ranking.value = emptyList()
            _session.value = Session.LoggedOut
            _message.value = "账号已注销"
            onDone()
        } else {
            _message.value = resp.error ?: "注销失败"
        }
    }

    fun logout() {
        viewModelScope.launch {
            // 先通知服务端吊销 refresh（失败也要放行本地登出，否则断网就退不出去）
            runCatching { api.logout() }
            tokenStore.clear()
            _session.value = Session.LoggedOut
            _members.value = emptyList()
            _ranking.value = emptyList()
            clearScreenDetail()
        }
    }

    /** 屏幕页数据缓存随会话清掉：换账号后不能看到上一个人的缓存（12.2.7a：进页只在无缓存时读取）。 */
    private fun clearScreenDetail() {
        _screenDetail.value = null
        _memberDetail.value = null
        _familyScreen.value = null
    }

    // ---------------- 概览 ----------------

    /** 拉取屏幕时间结算历史。判定（是否达标、扣多少分）全部以服务端返回为准。 */
    fun loadScreenReports() = launchBusy(showBusy = false) {
        runCatching { api.screenReports(limit = 14).data?.items.orEmpty() }
            .onSuccess { _screenReports.value = it }
    }

    private val _icp = MutableStateFlow("")
    val icp: StateFlow<String> = _icp.asStateFlow()

    fun loadOverview() = launchBusy(showBusy = false) {
        val st = api.state().data ?: return@launchBusy
        _icp.value = st.icp
        _familyName.value = st.family.name
        _members.value = st.members.filter { !it.hidden }
        _unread.value = st.unreadCount
        _transferMax.value = st.transferMax
        // 顺手把 me 的分数刷新到会话里
        _session.value = _session.value.let {
            when (it) {
                is Session.Ready -> Session.Ready(it.me.copy(score = st.me.score, frozenScore = st.me.frozenScore, role = st.me.role))
                is Session.NoFamily -> Session.NoFamily(it.me.copy(score = st.me.score))
                else -> it
            }
        }
    }

    fun loadRanking(type: String, period: String = "week") = launchBusy(showBusy = false) {
        val resp = api.ranking(type, period)
        _ranking.value = resp.data?.rows.orEmpty()
    }

    /** 不传 member_id：后端按全组聚合，普通成员也不会触发 403（查他人才需要组长）。 */
    fun loadTrend(period: String = "week") = launchBusy(showBusy = false) {
        _trend.value = api.trend(period = period).data?.points.orEmpty()
    }

    fun loadRecords(memberId: Long? = null, keyword: String? = null) = launchBusy(showBusy = false) {
        _records.value = api.records(memberId = memberId, keyword = keyword, limit = 100).data?.items.orEmpty()
    }

    fun loadItems() = launchBusy(showBusy = false) {
        _items.value = api.items().data?.items.orEmpty()
    }

    fun loadTasks() = launchBusy(showBusy = false) {
        _tasks.value = api.tasks().data?.tasks.orEmpty()
    }

    fun loadTaskDetail(taskId: Long) = launchBusy(showBusy = false) {
        _taskDetail.value = api.taskDetail(taskId).data
    }

    fun loadNotifications() = launchBusy(showBusy = false) {
        _notifications.value = api.notifications().data?.items.orEmpty()
    }

    fun loadRules() = launchBusy(showBusy = false) {
        _rules.value = api.rules().data?.items.orEmpty()
        _screenRules.value = api.screenRules().data?.items.orEmpty()
    }

    /** 家庭码隐藏状态（2026-11-06）。 */
    private val _codeHidden = MutableStateFlow(false)
    val codeHidden: StateFlow<Boolean> = _codeHidden.asStateFlow()

    fun loadInviteCode() = launchBusy(showBusy = false) {
        val d = api.familyCode().data
        _inviteCode.value = d?.code
        _codeHidden.value = d?.hidden ?: false
    }

    /** 组长设置家庭码是否可被搜索到。码本身永不变更。 */
    fun setCodeHidden(hidden: Boolean) = launchBusy {
        val resp = api.setCodeHidden(CodeHiddenReq(hidden))
        if (resp.ok) {
            _codeHidden.value = resp.data?.hidden ?: hidden
            _message.value = if (hidden) "家庭码已隐藏，别人搜不到" else "家庭码已公开，可被搜到"
        } else {
            _message.value = resp.error ?: "设置失败"
        }
    }

    // ---------------- 未入组者：搜索 / 建组 / 申请（2026-11-06） ----------------

    private val _familySearch = MutableStateFlow<FamilySearchView?>(null)
    val familySearch: StateFlow<FamilySearchView?> = _familySearch.asStateFlow()

    private val _myJoinRequests = MutableStateFlow<List<InvitationView>>(emptyList())
    val myJoinRequests: StateFlow<List<InvitationView>> = _myJoinRequests.asStateFlow()

    /** 本组待审批的入组申请（组长）。 */
    private val _pendingRequests = MutableStateFlow<List<InvitationView>>(emptyList())
    val pendingRequests: StateFlow<List<InvitationView>> = _pendingRequests.asStateFlow()

    fun clearFamilySearch() { _familySearch.value = null }

    fun searchFamily(code: String) = launchBusy {
        val resp = api.searchFamily(code.trim().uppercase())
        if (!resp.ok) {
            _message.value = resp.error ?: "搜索失败"
            return@launchBusy
        }
        _familySearch.value = resp.data
    }

    fun requestJoin(code: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.requestJoin(RequestJoinReq(code.trim().uppercase()))
        if (resp.ok) {
            _message.value = "已提交申请，等待组长审批"
            loadMyJoinRequests()
            onDone()
        } else {
            _message.value = resp.error ?: "申请失败"
        }
    }

    fun createFamily(name: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.createFamily(CreateFamilyReq(name.trim()))
        if (resp.ok) {
            _message.value = "已创建「${resp.data?.name ?: ""}」"
            refreshAfterFamilyChange()
            onDone()
        } else {
            _message.value = resp.error ?: "创建失败"
        }
    }

    fun loadMyJoinRequests() = launchBusy(showBusy = false) {
        _myJoinRequests.value = api.myJoinRequests().data?.items.orEmpty()
    }

    fun loadPendingRequests() = launchBusy(showBusy = false) {
        _pendingRequests.value = api.pendingJoinRequests().data?.items.orEmpty()
    }

    fun decideJoinRequest(id: Long, accept: Boolean) = launchBusy {
        val resp = api.decideJoinRequest(DecideRequestReq(id, accept))
        _message.value = if (resp.ok) {
            if (accept) "已同意加入" else "已拒绝"
        } else {
            resp.error ?: "操作失败"
        }
        loadPendingRequests()
        loadOverview()
    }

    /** 家庭关系变了（入组/建组/退出）之后统一刷新身份与概览。 */
    fun refreshAfterFamilyChange() {
        viewModelScope.launch {
            runCatching { api.me().data }.getOrNull()?.let { me ->
                _session.value = Session.Ready(me)
            }
            loadOverview()
        }
    }

    // ---------------- 待确认邀请（P3.1） ----------------
    // 与邀请码是两条独立入组路径：邀请码是自助凭码入组，这里必须经被邀请人确认。

    private val _myInvitations = MutableStateFlow<List<InvitationView>>(emptyList())
    val myInvitations: StateFlow<List<InvitationView>> = _myInvitations.asStateFlow()

    private val _familyInvitations = MutableStateFlow<List<InvitationView>>(emptyList())
    val familyInvitations: StateFlow<List<InvitationView>> = _familyInvitations.asStateFlow()

    /** 未入组也能调：接口挂在 /api/app 而不是 RequireFamily 的组内路由。 */
    fun loadMyInvitations() = launchBusy(showBusy = false) {
        _myInvitations.value = api.myInvitations().data?.items.orEmpty()
    }

    fun loadFamilyInvitations() = launchBusy(showBusy = false) {
        _familyInvitations.value = api.familyInvitations().data?.items.orEmpty()
    }

    fun inviteByUsername(username: String) = launchBusy {
        val resp = api.inviteByUsername(InviteByUsernameReq(username.trim()))
        _message.value = if (resp.ok) "已发出邀请，等待对方确认" else (resp.error ?: "邀请失败")
        if (resp.ok) loadFamilyInvitations()
    }

    fun cancelInvitation(invitationId: Long) = launchBusy {
        val resp = api.cancelInvitation(InvitationIdReq(invitationId))
        _message.value = if (resp.ok) "已取消邀请" else (resp.error ?: "取消失败")
        if (resp.ok) loadFamilyInvitations()
    }

    fun decideInvitation(invitationId: Long, accept: Boolean) = launchBusy {
        val resp = if (accept) {
            api.acceptInvitation(InvitationIdReq(invitationId))
        } else {
            api.rejectInvitation(InvitationIdReq(invitationId))
        }
        if (!resp.ok) {
            _message.value = resp.error ?: "操作失败"
            return@launchBusy
        }
        loadMyInvitations()
        if (!accept) {
            _message.value = "已拒绝邀请"
            return@launchBusy
        }
        // 接受后服务端已把 family_id 填上。必须重新拉 me、刷新本地 familyId 并切到 Ready，
        // 否则界面还停在 NoFamily，且后续组内请求会带上旧身份。
        val me = api.me().data
        if (me != null) {
            tokenStore.saveSession(
                tokenStore.accessTokenOnce().orEmpty(),
                tokenStore.refreshTokenOnce().orEmpty(),
                me.id, me.nickname, me.role, me.familyId,
            )
            _session.value = Session.Ready(me)
            loadOverview()
            scheduler.runNow()
        }
        _message.value = "已加入家庭组"
    }

    // ---------------- 商品券 ----------------

    fun loadCoupons() = launchBusy(showBusy = false) {
        _coupons.value = api.coupons().data?.items.orEmpty()
        _pendingCoupons.value = api.pendingCoupons().data?.items.orEmpty()
    }

    /** 使用券：发起核销投票。1 人家庭会被服务端直接核销。 */
    fun useCoupon(couponId: Long) = launchBusy {
        val resp = api.useCoupon(CouponIdReq(couponId))
        val c = resp.data
        _message.value = when {
            !resp.ok -> resp.error ?: "操作失败"
            c?.status == "redeemed" -> "家庭只有你一人，已直接核销"
            else -> "已发起核销，等待家人投票"
        }
        loadCoupons()
    }

    fun voteCoupon(couponId: Long, vote: String) = launchBusy {
        val resp = api.voteCoupon(CouponVoteReq(couponId, vote))
        val r = resp.data
        _message.value = when {
            !resp.ok -> resp.error ?: "投票失败"
            r?.status == "redeemed" -> "过半同意，券已核销"
            r?.status == "unused" -> "反对票足够，核销被驳回"
            else -> "已投票（同意 ${r?.approve ?: 0}）"
        }
        loadCoupons()
    }

    // ---------------- 积分动作 ----------------

    fun redeem(itemId: Long) = launchBusy {
        val resp = api.redeem(RedeemReq(itemId))
        _message.value = if (resp.ok) "兑换成功" else (resp.error ?: "兑换失败")
        if (resp.ok) { loadOverview(); loadItems(); loadCoupons() }
    }

    fun sendTransfer(toMemberId: Long, amount: Int, note: String) = launchBusy {
        val resp = api.sendTransfer(TransferReq(toMemberId, amount, note))
        _message.value = if (resp.ok) "已赠送" else (resp.error ?: "赠送失败")
        if (resp.ok) loadOverview()
    }

    // ---------------- 任务 ----------------

    fun createTask(title: String, points: Int) = launchBusy {
        val resp = api.createTask(CreateTaskReq(title.trim(), points))
        _message.value = if (resp.ok) "任务已发布，积分已冻结" else (resp.error ?: "发布失败")
        if (resp.ok) { loadTasks(); loadOverview() }
    }

    fun claimTask(taskId: Long) = launchBusy {
        val resp = api.claimTask(TaskIdReq(taskId))
        _message.value = if (resp.ok) "已接取" else (resp.error ?: "接取失败")
        if (resp.ok) { loadTasks(); loadTaskDetail(taskId) }
    }

    fun abandonTask(taskId: Long) = launchBusy {
        val resp = api.abandonTask(TaskIdReq(taskId))
        _message.value = if (resp.ok) "已放弃" else (resp.error ?: "操作失败")
        if (resp.ok) { loadTasks(); loadTaskDetail(taskId) }
    }

    fun closeTask(taskId: Long) = launchBusy {
        val resp = api.closeTask(TaskIdReq(taskId))
        _message.value = if (resp.ok) "任务已关闭，冻结积分已退回" else (resp.error ?: "操作失败")
        if (resp.ok) { loadTasks(); loadTaskDetail(taskId); loadOverview() }
    }

    fun submitTask(taskId: Long, note: String) = launchBusy {
        val resp = api.submitTask(SubmitReq(taskId, note.trim()))
        _message.value = if (resp.ok) "已提交，等待投票" else (resp.error ?: "提交失败")
        if (resp.ok) { loadTaskDetail(taskId); loadTasks() }
    }

    fun voteTask(submissionId: Long, vote: String, taskId: Long) = launchBusy {
        val resp = api.voteTask(VoteReq(submissionId, vote))
        val d = resp.data
        _message.value = when {
            !resp.ok -> resp.error ?: "投票失败"
            d?.status == "approved" -> "投票通过，积分已转移"
            d?.status == "rejected" -> "任务被驳回"
            else -> "已投票（同意 ${d?.approve ?: 0}/${d?.voters ?: 0}）"
        }
        loadTaskDetail(taskId)
        loadTasks()
        loadOverview()
    }

    // ---------------- 通知 ----------------

    fun readNotification(id: Long) = launchBusy(showBusy = false) {
        api.readNotification(NotificationReadReq(id = id))
        loadNotifications()
        loadOverview()
    }

    fun readAllNotifications() = launchBusy(showBusy = false) {
        api.readNotification(NotificationReadReq(all = true))
        loadNotifications()
        loadOverview()
    }

    // ---------------- 商城管理（组长） ----------------

    fun saveItem(id: Long?, name: String, cost: Int) = launchBusy {
        val resp = api.saveItem(ItemSaveReq(id, name.trim(), cost))
        _message.value = if (resp.ok) "已保存" else (resp.error ?: "保存失败")
        if (resp.ok) loadItems()
    }

    fun deleteItem(id: Long) = launchBusy {
        val resp = api.deleteItem(ItemIdReq(id))
        _message.value = if (resp.ok) "已删除" else (resp.error ?: "删除失败")
        if (resp.ok) loadItems()
    }

    // ---------------- 成员管理（组长） ----------------

    fun kickMember(memberId: Long) = launchBusy {
        val resp = api.kickMember(MemberIdReq(memberId, confirm = true))
        _message.value = if (resp.ok) "已踢出，积分清零" else (resp.error ?: "操作失败")
        if (resp.ok) loadOverview()
    }

    fun setMemberHidden(memberId: Long, hidden: Boolean) = launchBusy {
        val resp = api.setHidden(HiddenReq(memberId, hidden))
        _message.value = if (resp.ok) (if (hidden) "已隐藏" else "已显示") else (resp.error ?: "操作失败")
        if (resp.ok) loadOverview()
    }

    fun renameMember(memberId: Long, nickname: String) = launchBusy {
        val resp = api.renameMember(RenameMemberReq(memberId, nickname.trim()))
        _message.value = if (resp.ok) "已改昵称" else (resp.error ?: "改昵称失败")
        if (resp.ok) loadOverview()
    }

    fun transferLeader(memberId: Long) = launchBusy {
        val resp = api.transferLeader(TransferLeaderReq(memberId))
        _message.value = if (resp.ok) "已变更组长" else (resp.error ?: "操作失败")
        if (resp.ok) {
            val me = api.me().data
            if (me != null) {
                tokenStore.updateRole(me.role)
                _session.value = Session.Ready(me)
            }
            loadOverview()
        }
    }

    fun leaveFamily() = launchBusy {
        val resp = api.leaveFamily(com.familypoints.app.data.api.ConfirmReq(true))
        if (resp.ok) {
            _message.value = "已退出家庭组，积分清零"
            val me = api.me().data
            if (me != null) _session.value = Session.NoFamily(me)
        } else {
            _message.value = resp.error ?: "退出失败"
        }
    }

    // ---------------- 睡觉规则（组长） ----------------

    fun saveScreenRule(rule: ScreenRuleSaveReq) = launchBusy {
        val resp = api.saveScreenRule(rule)
        _message.value = if (resp.ok) "规则已保存" else (resp.error ?: "保存失败")
        if (resp.ok) loadRules()
    }

    fun saveStreakTiers(ruleId: Long?, name: String, points: Int, tiers: List<StreakTier>) = launchBusy {
        val resp = api.saveRule(
            RuleSaveReq(id = ruleId, name = name, points = points, triggerType = "screen_night", streaks = tiers)
        )
        _message.value = if (resp.ok) "连续奖励已保存" else (resp.error ?: "保存失败")
        if (resp.ok) loadRules()
    }

    // ---------------- 工具 ----------------

    fun consumeMessage() { _message.value = null }

    fun showMessage(msg: String) { _message.value = msg }

    // ---------------- 家庭屏幕详情（2026-11-05） ----------------

    /** 全组当天屏幕时间：每人汇总 + 各设备明细。 */
    private val _familyScreen = MutableStateFlow<FamilyScreenView?>(null)
    val familyScreen: StateFlow<FamilyScreenView?> = _familyScreen.asStateFlow()

    fun loadFamilyScreen(date: String? = null) = launchBusy(showBusy = false) {
        runCatching { api.screenFamily(date).data }
            .onSuccess { _familyScreen.value = it }
            .onFailure { Log.w(TAG, "读取家庭屏幕详情失败", it) }
    }

    // ---------------- 屏幕时间明细（2026-11-06） ----------------

    /** 自己的明细（屏幕 tab 用）。与「别人的」分开存，避免来回切页互相覆盖。 */
    private val _screenDetail = MutableStateFlow<MemberDetail?>(null)
    val screenDetail: StateFlow<MemberDetail?> = _screenDetail.asStateFlow()

    /** 某个家庭成员的明细（成员详情页用）。 */
    private val _memberDetail = MutableStateFlow<MemberDetail?>(null)
    val memberDetail: StateFlow<MemberDetail?> = _memberDetail.asStateFlow()

    /**
     * 读取某成员某天的明细（总时长 / 24 小时桶 / 各设备 / 近 7 天）。
     * 服务端已算好全部维度，前端只画，不做任何补零或重算。
     *
     * memberId = 0 表示自己，写入 [screenDetail]；其余写入 [memberDetail]。
     */
    fun loadScreenDetail(memberId: Long, date: String? = null) = launchBusy(showBusy = false) {
        if (memberId == 0L) _screenDetail.value = null
        else _memberDetail.value = null
        runCatching { api.screenMember(memberId, date).data }
            .onSuccess { d ->
                if (memberId == 0L) _screenDetail.value = d else _memberDetail.value = d
            }
            .onFailure { Log.w(TAG, "读取成员屏幕明细失败 member=$memberId", it) }
    }

    /**
     * 上传「今天」的实时快照（只落明细，不结算、不发分）。
     *
     * 与 Worker 的区别：Worker 上报「昨天」并结算；快照只为了让家人现在就能看到
     * 今天的数据。失败静默——这是旁路，不能影响页面。
     */
    suspend fun uploadTodaySnapshot(
        deviceKey: String,
        deviceModel: String,
        hours: IntArray,
        nightMin: Int,
        date: String,
    ) {
        if (hours.size != 24) return
        runCatching {
            api.screenSnapshot(
                ScreenReportReq(
                    deviceKey = deviceKey,
                    deviceModel = deviceModel,
                    reportDate = date,
                    totalMin = hours.sum(),
                    nightMin = nightMin,
                    hourly = hours.toList(),
                )
            )
        }.onFailure { Log.w(TAG, "上传今日快照失败", it) }
    }

    // ---------------- 家庭组（2026-11-06） ----------------

    /** 改家庭组名（仅组长）。成功后同步本地的 familyName，无需重新拉接口。 */
    fun renameFamily(name: String, onDone: () -> Unit = {}) = launchBusy {
        val trimmed = name.trim()
        val resp = api.renameFamily(FamilyRenameReq(trimmed))
        if (!resp.ok) {
            showMessage(resp.error ?: "保存失败")
            return@launchBusy
        }
        _familyName.value = trimmed
        showMessage("家庭名称已保存")
        onDone()
    }

    // ---------------- 个人资料 ----------------

    /** 改密码：成功后该账号全部 refresh 失效，必须重新登录（后端会吊销）。 */
    fun changePassword(oldPwd: String, newPwd: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.changePassword(PasswordReq(oldPassword = oldPwd, newPassword = newPwd))
        if (!resp.ok) {
            showMessage(resp.error ?: "修改失败")
            return@launchBusy
        }
        showMessage("密码已修改，请用新密码重新登录")
        tokenStore.clear()
        _session.value = Session.LoggedOut
        onDone()
    }


    /** 改自己的昵称 / 头像；成功后同步刷新 session 里的 me。 */
    fun updateProfile(nickname: String, avatar: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.updateProfile(ProfileReq(nickname = nickname, avatar = avatar))
        if (!resp.ok) {
            showMessage(resp.error ?: "保存失败")
            return@launchBusy
        }
        val me = resp.data
        if (me != null) {
            val cur = (_session.value as? Session.Ready)?.me
            if (cur != null) _session.value = Session.Ready(me.copy(familyId = cur.familyId ?: me.familyId))
        }
        showMessage("已保存")
        loadOverview()
        onDone()
    }

    // 返回 Job：下拉刷新需要 join 等待完成（2026-11-07）
    private fun launchBusy(showBusy: Boolean = true, block: suspend () -> Unit) = viewModelScope.launch {
        if (showBusy) _busy.value = true
        try {
            block()
        } catch (e: Exception) {
            _message.value = e.message ?: "网络错误"
        } finally {
            if (showBusy) _busy.value = false
        }
    }

    // ---------- 通知未读轮询（2026-11-07：全 App 唯一的后台定时请求） ----------

    private var unreadPollJob: Job? = null

    /** 每分钟查一次未读数；只在已入组的登录态下真正发请求。 */
    fun startUnreadPolling() {
        if (unreadPollJob?.isActive == true) return
        unreadPollJob = viewModelScope.launch {
            while (isActive) {
                delay(60_000)
                if (session.value !is Session.Ready) continue
                runCatching { api.state() }.onSuccess { st ->
                    st.data?.let { _unread.value = it.unreadCount }
                }
            }
        }
    }


    // ---------- 设备绑定与解绑（2026-11-07，docs 15.5） ----------

    private val _myDevices = MutableStateFlow<List<MemberDeviceView>>(emptyList())
    val myDevices: StateFlow<List<MemberDeviceView>> = _myDevices.asStateFlow()

    private val _myUnbindRequests = MutableStateFlow<List<UnbindRequestView>>(emptyList())
    val myUnbindRequests: StateFlow<List<UnbindRequestView>> = _myUnbindRequests.asStateFlow()

    private val _pendingUnbindRequests = MutableStateFlow<List<UnbindRequestView>>(emptyList())
    val pendingUnbindRequests: StateFlow<List<UnbindRequestView>> = _pendingUnbindRequests.asStateFlow()

    /** 设备管理页数据：我的设备 + 我的申请（组长再加待审批）。 */
    fun loadDeviceManage() = launchBusy(showBusy = false) {
        _myDevices.value = api.myDevices().data?.devices.orEmpty()
        _myUnbindRequests.value = api.myUnbindRequests().data?.items.orEmpty()
        val leader = (session.value as? Session.Ready)?.me?.role == "leader"
        if (leader) {
            _pendingUnbindRequests.value = api.pendingUnbindRequests().data?.items.orEmpty()
        }
    }

    /** 组长直接解绑。 */
    fun unbindDevice(deviceKey: String) = launchBusy {
        val resp = api.unbindDevice(DeviceKeyReq(deviceKey))
        _message.value = resp.data?.takeIf { it.unbound }?.let { "已解绑" } ?: (resp.error ?: "解绑失败")
        loadDeviceManage()
    }

    /** 普通成员：发起解绑申请，等组长审批。 */
    fun requestUnbindDevice(deviceKey: String) = launchBusy {
        val resp = api.requestUnbind(DeviceKeyReq(deviceKey))
        if (resp.ok) _message.value = "解绑申请已提交，等待组长审批"
        else _message.value = resp.error ?: "提交失败"
        loadDeviceManage()
    }

    /** 组长审批解绑申请：同意即立即解绑。 */
    fun decideUnbind(requestId: Long, approve: Boolean) = launchBusy {
        api.decideUnbind(DecideUnbindReq(requestId, approve))
        _message.value = if (approve) "已同意解绑" else "已拒绝"
        loadDeviceManage()
    }

    /** 撤回自己的解绑申请。 */
    fun cancelUnbind(requestId: Long) = launchBusy {
        api.cancelUnbind(RequestIdReq(requestId))
        _message.value = "已撤回申请"
        loadDeviceManage()
    }
}
