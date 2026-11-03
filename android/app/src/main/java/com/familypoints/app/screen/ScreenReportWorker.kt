package com.familypoints.app.screen

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.DeviceReportReq
import com.familypoints.app.data.local.TokenStore
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * 屏幕时间上报 Worker。
 *
 * 上报的是「昨天」的完整数据：昨天 0 点到现在这一天的夜间段都已结束，
 * 数字是确定的，不会因为当天还在用手机而反复变化。
 *
 * 幂等由服务端保证：`UNIQUE(member_id, report_date)` + 后报覆盖。
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

        val deviceToken = tokenStore.deviceTokenOnce()
        if (deviceToken.isNullOrBlank()) return Result.success()

        // 夜间时段配置来自服务端；拿不到就用 0–7 点的默认值
        val rule = runCatching { apiClient.service.screenRules().data?.items?.firstOrNull() }.getOrNull()
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
            val resp = apiClient.service.deviceReport(
                deviceToken,
                DeviceReportReq(
                    reportDate = reportDate,
                    totalMin = total.totalMinutes,
                    nightMin = nightMinutes,
                )
            )
            if (resp.ok) Result.success() else Result.retry()
        }.getOrElse { Result.retry() }
    }
}
