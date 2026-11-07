package com.familypoints.app.screen

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.familypoints.app.data.api.ApiClient
import com.familypoints.app.data.api.ScreenReportReq
import com.familypoints.app.util.DeviceInfo
import com.familypoints.app.data.local.ReportStatusStore
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
    private val statusStore: ReportStatusStore,
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

        // 一次读 8 天（D-7..D-1 七个完整日 + 今天）的小时桶：
        // - D-7..D-1：**补算最近 7 天**（12.2.5）——设备关机几天也能在下一次运行时补齐；
        //   事件数为 0 的天跳过（可能是系统没保留那么久的数据，不能拿 0 覆盖真实结算）；
        //   服务端「数值未变直接返回」，重复上报不会重复计分；
        // - 今天：快照 + 跨零点夜间时段（22 → 7 的 0–7 点落在今天）。
        // 总时长直接取桶之和，保证「总时长」与「逐小时」永远对得上。
        val buckets = reader.readBuckets(8)
        if (buckets.size < 8) return Result.success()
        val today = buckets[7]

        // 1) 逐日上报 D-7..D-1 并结算（幂等：member+device+date，后报覆盖）
        for (i in 0..6) {
            val day = buckets[i]
            if (day.eventCount == 0) continue
            val startedAt = System.currentTimeMillis()
            // 夜间段：同一天内直接取；跨零点（如 22 → 7）要把次日 0 点到 nightEnd 也算上
            val nightMinutes = if (nightStart != null && nightEnd != null) {
                if (nightStart < nightEnd) {
                    day.nightMinutes(nightStart, nightEnd)
                } else {
                    day.nightMinutes(nightStart, 24) + buckets[i + 1].nightMinutes(0, nightEnd)
                }
            } else {
                0
            }
            val outcome = runCatching {
                apiClient.service.screenReport(
                    ScreenReportReq(
                        deviceKey = DeviceInfo.key(applicationContext),
                        deviceModel = DeviceInfo.model(),
                        reportDate = day.date,
                        totalMin = day.totalMinutes,
                        nightMin = nightMinutes,
                        hourly = day.hourlyMinutes.toList(),
                    )
                ).ok
            }
            outcome.fold(
                onSuccess = { ok ->
                    if (ok) statusStore.recordSuccess(startedAt)
                    else statusStore.recordFailure(startedAt, "服务端拒绝上报")
                },
                onFailure = { e ->
                    statusStore.recordFailure(startedAt, e.message ?: e.javaClass.simpleName)
                },
            )
            if (outcome.getOrNull() != true) return Result.retry()
        }

        // 2) 顺带把「今天」的快照也传一次：家人现在就能看到今天的数据。
        //    快照不结算、不发分，失败也不影响上面那次上报的结果。
        runCatching {
            apiClient.service.screenSnapshot(
                ScreenReportReq(
                    deviceKey = DeviceInfo.key(applicationContext),
                    deviceModel = DeviceInfo.model(),
                    reportDate = today.date,
                    totalMin = today.totalMinutes,
                    nightMin = today.nightMinutes(0, 24),
                    hourly = today.hourlyMinutes.toList(),
                )
            )
        }

        return Result.success()
    }
}
