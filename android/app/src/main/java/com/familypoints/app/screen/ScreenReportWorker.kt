package com.familypoints.app.screen

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.ScreenReportReq
import com.familypoints.app.util.DeviceInfo
import com.familypoints.app.data.local.TokenStore
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * 屏幕时间上报 Worker。
 *
 * 上报的是「昨天」的完整数据：昨天 0 点到现在这一天的夜间段都已结束，
 * 数字是确定的，不会因为当天还在用手机而反复变化。
 *
 * 幂等由服务端保证：`UNIQUE(member_id, device_key, report_date)` + 后报覆盖；
 * 多台设备各自上报，服务端按「成员×日」相加后再结算。
 * 所以这个 Worker 重试是安全的，失败重试不会重复计分。
 */
@HiltWorker
class ScreenReportWorker @AssistedInject constructor(
    @Assisted context: Context,
    @Assisted params: WorkerParameters,
    private val reader: UsageStatsReader,
    private val apiClient: ApiClient,
    private val tokenStore: TokenStore,
) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        // 没授权就没法采集，直接成功返回——否则会无限重试
        if (!reader.hasPermission()) return Result.success()

        // 2026-11-05：不再需要设备令牌，登录态即可上报
        if (tokenStore.accessTokenOnce().isNullOrBlank()) return Result.success()

        // 夜间时段配置来自服务端，**取本机登录成员自己的那条**。
        // 原来直接 firstOrNull()，等于随便抓一条别人的规则——多成员家庭会算错夜间判定。
        val myId = tokenStore.memberIdOnce() ?: 0L
        val rule = runCatching { apiClient.service.screenRules().data?.items }
            .onFailure { android.util.Log.w("ScreenReport", "读取屏幕规则失败", it) }
            .getOrNull()
            ?.let { list -> list.firstOrNull { it.memberId == myId } ?: list.firstOrNull() }
        val nightStart = rule?.nightStartHour
        val nightEnd = rule?.nightEndHour
        val nightMax = rule?.nightMaxMin

        val reportDate = reader.dateString(1) // 昨天
        val dayStart = reader.startOfDay(1)
        val dayEnd = reader.startOfDay(0)

        val total = reader.read(dayStart, dayEnd, nightStart, nightEnd, nightMax)

        // 夜间段：若跨零点（如 22 → 7），区间延伸到今天 7 点
        val nightMinutes = if (nightStart != null && nightEnd != null) {
            if (nightStart < nightEnd) {
                // 同一天内，total 里已经算过
                total.nightMinutes
            } else {
                // 跨零点：昨天 nightStart ~ 今天 nightEnd
                val crossEnd = reader.startOfDay(0) + nightEnd * 3_600_000L
                val cross = reader.read(dayStart, crossEnd, nightStart, nightEnd, nightMax)
                cross.nightMinutes
            }
        } else {
            0
        }

        return runCatching {
            val resp = apiClient.service.screenReport(
                ScreenReportReq(
                    deviceKey = DeviceInfo.key(applicationContext),
                    deviceModel = DeviceInfo.model(),
                    reportDate = reportDate,
                    totalMin = total.totalMinutes,
                    nightMin = nightMinutes,
                )
            )
            if (resp.ok) Result.success() else Result.retry()
        }.getOrElse { Result.retry() }
    }
}
