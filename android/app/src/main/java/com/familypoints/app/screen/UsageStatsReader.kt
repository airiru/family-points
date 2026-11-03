package com.familypoints.app.screen

import android.app.AppOpsManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.os.Build
import android.os.Process
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.Calendar
import javax.inject.Inject
import javax.inject.Singleton

/** 一天的用量结果。 */
data class UsageResult(
    val totalMinutes: Int,
    val nightMinutes: Int,
)

/**
 * 读取全机使用时长。
 *
 * **不用 `queryUsageStats(INTERVAL_DAILY)`**：它按自然日切桶，
 * 跨天查询会丢数据或重复（docs 12.2.2 明确要求）。
 * 这里用 `queryEvents` 配对 ACTIVITY_RESUMED / ACTIVITY_PAUSED 自己累加，口径可控。
 *
 * 同时排除：
 * - 本应用自己（否则上报动作本身会被计入）
 * - 桌面 / 系统 UI / 设置等系统界面
 */
@Singleton
class UsageStatsReader @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val usm: UsageStatsManager
        get() = context.getSystemService(Context.USAGE_STATS_SERVICE) as UsageStatsManager

    /** 是否已授予「使用情况访问」权限。这个权限只能由用户在系统设置里手动开启。 */
    fun hasPermission(): Boolean {
        val appOps = context.getSystemService(Context.APP_OPS_SERVICE) as AppOpsManager
        val mode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            appOps.unsafeCheckOpNoThrow(
                AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName
            )
        } else {
            @Suppress("DEPRECATION")
            appOps.checkOpNoThrow(
                AppOpsManager.OPSTR_GET_USAGE_STATS, Process.myUid(), context.packageName
            )
        }
        return mode == AppOpsManager.MODE_ALLOWED
    }

    /**
     * 统计 [from, to) 区间内的前台使用时长。
     *
     * @param nightStartHour 夜间时段起始小时（0–23），与 nightEndHour 一起界定「睡觉时段」
     * @param nightEndHour   夜间时段结束小时（0–23），可跨零点
     */
    fun read(from: Long, to: Long, nightStartHour: Int?, nightEndHour: Int?, nightMaxMin: Int?): UsageResult {
        if (!hasPermission()) return UsageResult(0, 0)

        val excluded = excludedPackages()
        val events = usm.queryEvents(from, to) ?: return UsageResult(0, 0)

        var totalMs = 0L
        var nightMs = 0L
        var foregroundPkg: String? = null
        var foregroundSince = 0L

        val ev = UsageEvents.Event()
        while (events.hasNextEvent()) {
            events.getNextEvent(ev)
            val ts = ev.timeStamp
            when (ev.eventType) {
                UsageEvents.Event.ACTIVITY_RESUMED -> {
                    // 上一次还没结算就切走了：结算到此刻
                    if (foregroundPkg != null && foregroundSince > 0) {
                        val d = ts - foregroundSince
                        if (d > 0 && foregroundPkg?.let { it !in excluded } == true) {
                            totalMs += d
                            if (isNight(foregroundSince, nightStartHour, nightEndHour)) nightMs += d
                        }
                    }
                    foregroundPkg = ev.packageName
                    foregroundSince = ts
                }

                UsageEvents.Event.ACTIVITY_PAUSED, UsageEvents.Event.ACTIVITY_STOPPED -> {
                    if (foregroundPkg != null && ev.packageName == foregroundPkg && foregroundSince > 0) {
                        val d = ts - foregroundSince
                        if (d > 0 && foregroundPkg?.let { it !in excluded } == true) {
                            totalMs += d
                            if (isNight(foregroundSince, nightStartHour, nightEndHour)) nightMs += d
                        }
                        foregroundPkg = null
                        foregroundSince = 0
                    }
                }
            }
        }
        // 区间结束时仍在前台：结算到区间末尾
        if (foregroundPkg != null && foregroundSince > 0) {
            val d = to - foregroundSince
            if (d > 0 && foregroundPkg?.let { it !in excluded } == true) {
                totalMs += d
                if (isNight(foregroundSince, nightStartHour, nightEndHour)) nightMs += d
            }
        }

        return UsageResult(
            totalMinutes = (totalMs / 60_000L).toInt().coerceAtLeast(0),
            nightMinutes = (nightMs / 60_000L).toInt().coerceAtLeast(0),
        )
    }

    /** 某时刻是否落在夜间时段内。支持跨零点（如 22 → 7）。 */
    private fun isNight(ts: Long, startHour: Int?, endHour: Int?): Boolean {
        if (startHour == null || endHour == null || startHour == endHour) return false
        val cal = Calendar.getInstance().apply { timeInMillis = ts }
        val h = cal.get(Calendar.HOUR_OF_DAY)
        return if (startHour < endHour) {
            h in startHour until endHour
        } else {
            // 跨零点：22,23,0..6
            h >= startHour || h < endHour
        }
    }

    /** 排除桌面、系统 UI、设置等——它们不代表「玩手机」。 */
    private fun excludedPackages(): Set<String> = buildSet {
        add(context.packageName)
        add("com.android.systemui")
        add("com.android.settings")
        // 常见桌面
        add("com.android.launcher")
        add("com.android.launcher2")
        add("com.android.launcher3")
        add("com.google.android.apps.nexuslauncher")
        add("com.miui.home")
        add("com.huawei.android.launcher")
        add("com.oppo.launcher")
        add("com.vivo.launcher")
        add("com.bbk.launcher2")
    }

    /** 当天 0 点的毫秒时间戳。 */
    fun startOfDay(daysAgo: Int = 0): Long = Calendar.getInstance().apply {
        add(Calendar.DAY_OF_YEAR, -daysAgo)
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }.timeInMillis

    /** yyyy-MM-dd，用于上报的 report_date。 */
    fun dateString(daysAgo: Int = 0): String {
        val cal = Calendar.getInstance().apply { add(Calendar.DAY_OF_YEAR, -daysAgo) }
        return String.format(
            "%04d-%02d-%02d",
            cal.get(Calendar.YEAR), cal.get(Calendar.MONTH) + 1, cal.get(Calendar.DAY_OF_MONTH)
        )
    }
}
