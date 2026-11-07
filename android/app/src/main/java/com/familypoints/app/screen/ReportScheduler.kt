package com.familypoints.app.screen

import android.content.Context
import androidx.work.Constraints
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.Calendar
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton

/**
 * 上报调度。
 *
 * 双保险（docs 12.2.4 / 12.2.5）：
 * 1. WorkManager 每日一次——用户不打开 App 也能上报；
 * 2. App 打开时补算——国产 ROM 杀后台后，这是可靠性主要来源。
 *
 * 这里只负责 1；2 由 [ScreenSync] 在启动时调用。
 */
@Singleton
class ReportScheduler @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    companion object {
        // 2026-11-07 用户定稿：每天 10:00 / 18:00 两次定时上报；打开 App 补算一次；手动上报在屏幕页。
        private val HOURS = listOf(10, 18)

        private fun workName(hour: Int) = "screen_report_$hour"
        /** 旧版单一 21:00 任务的名称，schedule() 时顺手取消，避免升级后双份任务并存。 */
        private const val LEGACY_NAME = "screen_report_daily"
    }

    fun schedule() {
        val wm = WorkManager.getInstance(context)
        wm.cancelUniqueWork(LEGACY_NAME)
        for (hour in HOURS) {
            val now = Calendar.getInstance()
            val target = Calendar.getInstance().apply {
                set(Calendar.HOUR_OF_DAY, hour)
                set(Calendar.MINUTE, 0)
                set(Calendar.SECOND, 0)
                set(Calendar.MILLISECOND, 0)
                if (before(now)) add(Calendar.DAY_OF_YEAR, 1)
            }
            val request = PeriodicWorkRequestBuilder<ScreenReportWorker>(1, TimeUnit.DAYS)
                .setInitialDelay(target.timeInMillis - now.timeInMillis, TimeUnit.MILLISECONDS)
                .setConstraints(
                    Constraints.Builder()
                        .setRequiredNetworkType(NetworkType.CONNECTED)
                        .build()
                )
                .build()
            wm.enqueueUniquePeriodicWork(workName(hour), ExistingPeriodicWorkPolicy.UPDATE, request)
        }
    }

    /** 打开 App 时立即补算一次（不等待 21:00）。 */
    fun runNow() {
        val request = androidx.work.OneTimeWorkRequestBuilder<ScreenReportWorker>()
            .setConstraints(
                Constraints.Builder()
                    .setRequiredNetworkType(NetworkType.CONNECTED)
                    .build()
            )
            .build()
        WorkManager.getInstance(context).enqueue(request)
    }

    fun cancel() {
        val wm = WorkManager.getInstance(context)
        wm.cancelUniqueWork(LEGACY_NAME)
        for (hour in HOURS) wm.cancelUniqueWork(workName(hour))
    }
}
