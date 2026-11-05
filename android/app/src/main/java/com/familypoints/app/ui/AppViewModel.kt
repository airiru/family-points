package com.familypoints.app.ui

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.CouponIdReq
import com.familypoints.app.data.api.CouponView
import com.familypoints.app.data.api.CouponVoteReq
import com.familypoints.app.data.api.CreateTaskReq
import com.familypoints.app.data.api.FamilyScreenView
import com.familypoints.app.data.api.HiddenReq
import com.familypoints.app.data.api.ItemIdReq
import com.familypoints.app.data.api.ItemSaveReq
import com.familypoints.app.data.api.ItemView
import com.familypoints.app.data.api.JoinReq
import com.familypoints.app.data.api.LoginReq
import com.familypoints.app.data.api.MeView
import com.familypoints.app.data.api.MemberBrief
import com.familypoints.app.data.api.MemberIdReq
import com.familypoints.app.data.api.NotificationReadReq
import com.familypoints.app.data.api.NotificationView
import com.familypoints.app.data.api.PasswordReq
import com.familypoints.app.data.api.ProfileReq
import com.familypoints.app.data.api.RankingRow
import com.familypoints.app.data.api.RecordView
import com.familypoints.app.data.api.RedeemReq
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
    }

    // ---------------- 会话 ----------------

    private fun restoreSession() {
        viewModelScope.launch {
            val token = tokenStore.accessTokenOnce()
            if (token.isNullOrBlank()) {
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
                    MeView(id = id, name = "", familyId = familyId)
                )
            }
        }
    }

    fun login(username: String, password: String) = launchBusy {
        val resp = api.login(LoginReq(username.trim(), password))
        handleAuth(resp.data?.member, resp.data?.token?.accessToken, resp.data?.token?.refreshToken, resp.error)
    }

    fun register(username: String, password: String, name: String, createFamily: Boolean, familyName: String) =
        launchBusy {
            val resp = api.register(
                RegisterReq(
                    username = username.trim(),
                    password = password,
                    name = name.trim().ifBlank { username.trim() },
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
        tokenStore.saveSession(access, refresh, me.id, me.name, me.role, me.familyId)
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
                me.id, me.name, me.role, me.familyId,
            )
            _session.value = Session.Ready(me)
            loadOverview()
            scheduler.runNow()
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
        }
    }

    // ---------------- 概览 ----------------

    /** 拉取屏幕时间结算历史。判定（是否达标、扣多少分）全部以服务端返回为准。 */
    fun loadScreenReports() = launchBusy(showBusy = false) {
        runCatching { api.screenReports(limit = 14).data?.items.orEmpty() }
            .onSuccess { _screenReports.value = it }
    }

    fun loadOverview() = launchBusy(showBusy = false) {
        val st = api.state().data ?: return@launchBusy
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

    fun loadInviteCode() = launchBusy(showBusy = false) {
        _inviteCode.value = api.inviteCode().data?.inviteCode
    }

    fun resetInviteCode() = launchBusy {
        _inviteCode.value = api.resetInviteCode().data?.inviteCode
        _message.value = "邀请码已重置"
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

    fun renameMember(memberId: Long, name: String) = launchBusy {
        val resp = api.renameMember(RenameMemberReq(memberId, name.trim()))
        _message.value = if (resp.ok) "已改名" else (resp.error ?: "改名失败")
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


    /** 改自己的名称 / 头像；成功后同步刷新 session 里的 me。 */
    fun updateProfile(name: String, avatar: String, onDone: () -> Unit = {}) = launchBusy {
        val resp = api.updateProfile(ProfileReq(name = name, avatar = avatar))
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

    private fun launchBusy(showBusy: Boolean = true, block: suspend () -> Unit) {
        viewModelScope.launch {
            if (showBusy) _busy.value = true
            try {
                block()
            } catch (e: Exception) {
                _message.value = e.message ?: "网络错误"
            } finally {
                if (showBusy) _busy.value = false
            }
        }
    }
}
