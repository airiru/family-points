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
 * 某一天的用量明细（按小时分桶）。
 *
 * @property date yyyy-MM-dd
 * @property hourlyMinutes 长度 24，索引 = 小时（0–23），单位分钟
 * @property eventCount 当天事件条数（含被排除的应用）——
 *   0 = 这天系统里没有任何事件。补算时用它区分「设备没用过 / 数据未保留」，
 *   避免拿 0 覆盖服务端已有的真实结算。
 */
data class DayUsage(
    val date: String,
    val hourlyMinutes: IntArray,
    val eventCount: Int = 0,
) {
    val totalMinutes: Int get() = hourlyMinutes.sum()

    /** 夜间时段内的使用分钟数。支持跨零点（如 22 → 7）。 */
    fun nightMinutes(startHour: Int, endHour: Int): Int {
        if (startHour == endHour) return 0
        return if (startHour < endHour) {
            (startHour until endHour).sumOf { hourlyMinutes[it] }
        } else {
            ((startHour..23).toList() + (0 until endHour).toList()).sumOf { hourlyMinutes[it] }
        }
    }
}

/**
 * 读取全机使用时长。
 *
 * **不用 `queryUsageStats(INTERVAL_DAILY)`**：它按自然日切桶，
 * 跨天查询会丢数据或重复（docs 12.2.2 明确要求）。
 * 这里用 `queryEvents` 配对 ACTIVITY_RESUMED / ACTIVITY_PAUSED 自己累加，口径可控。
 *
 * 排除名单（2026-11-07 用户定稿）：**只有桌面**——本应用、系统 UI、设置等一律计入。
 */
@Singleton
class UsageStatsReader @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private companion object {
        const val HOUR_MS = 60L * 60 * 1000
    }

    private val usm: UsageStatsManager
        get() = context.getSystemService(Context.USAGE_STATS_SERVICE) as UsageStatsManager

    /**
     * 是否已授予「使用情况访问」权限。这个权限只能由用户在系统设置里手动开启。
     *
     * 不能只信 AppOps：部分 ROM 在系统层未真正放行时 AppOps 仍返回 allow
     * （此时 queryEvents 会抛安全异常），页面会误显示「已授权」却没有数据。
     * 所以这里再做一次功能性探测：真正能查到事件流才算数。
     */
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
        if (mode != AppOpsManager.MODE_ALLOWED) return false
        // 功能性探测：近 1 分钟没有事件是正常的（手机可能息屏），
        // 只关心「查询本身被系统拒绝」——那说明权限其实没生效。
        return runCatching {
            usm.queryEvents(System.currentTimeMillis() - 60_000L, System.currentTimeMillis())
            true
        }.getOrDefault(false)
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

    /**
     * 读取最近 [days] 天（含今天）的用量，按「天 × 小时」分桶。
     *
     * 只查一次 `queryEvents`，把每段前台时间按小时边界切开分别累加——
     * 逐天各查一次会重复扫描事件，24 小时再各查一次更慢。
     *
     * 返回列表**最旧在前、今天在最后**，便于直接画「近 7 天」。
     * 小时桶按「段起始时刻」归属：一段 06:50–07:10 会拆成 06 点 10 分 + 07 点 10 分。
     *
     * 说明：小时边界按 `dayStart + h × 1h` 推算，遇到有夏令时的地区，
     * 切换当天的某个小时会错位 1 小时（中国无夏令时，不影响本场景）。
     */
    fun readBuckets(days: Int): List<DayUsage> {
        val n = days.coerceIn(1, 31)
        if (!hasPermission()) return emptyList()

        val dayStarts = LongArray(n) { startOfDay(n - 1 - it) } // 最旧在前
        val windowFrom = dayStarts[0]
        val windowTo = System.currentTimeMillis()
        if (windowTo <= windowFrom) return emptyList()

        val ms = Array(n) { LongArray(24) }
        val evCount = IntArray(n)
        val excluded = excludedPackages()

        fun dayIndexOf(ts: Long): Int {
            // n ≤ 31，线性扫描足够；返回 -1 表示早于窗口，n 表示晚于窗口
            for (i in n - 1 downTo 0) {
                if (ts >= dayStarts[i]) return i
            }
            return -1
        }

        fun addInterval(rawStart: Long, rawEnd: Long) {
            var cur = maxOf(rawStart, windowFrom)
            val end = minOf(rawEnd, windowTo)
            while (cur < end) {
                val di = dayIndexOf(cur)
                if (di < 0) {
                    cur = dayStarts[0]
                    continue
                }
                if (di >= n) break
                val hour = ((cur - dayStarts[di]) / HOUR_MS).toInt().coerceIn(0, 23)
                val hourEnd = dayStarts[di] + (hour + 1) * HOUR_MS
                val segEnd = minOf(end, hourEnd)
                if (segEnd <= cur) { // 兜底：DST 等异常下避免死循环
                    cur += 1
                    continue
                }
                ms[di][hour] += segEnd - cur
                cur = segEnd
            }
        }

        val events = usm.queryEvents(windowFrom, windowTo) ?: return emptyList()
        var foregroundPkg: String? = null
        var foregroundSince = 0L
        val ev = UsageEvents.Event()

        while (events.hasNextEvent()) {
            events.getNextEvent(ev)
            val ts = ev.timeStamp
            // 事件计数按天归属（含被排除的应用）：补算时判断「这天有没有数据」
            dayIndexOf(ts).takeIf { it in 0 until n }?.let { evCount[it]++ }
            when (ev.eventType) {
                UsageEvents.Event.ACTIVITY_RESUMED -> {
                    if (foregroundPkg != null && foregroundSince > 0 &&
                        foregroundPkg?.let { it !in excluded } == true
                    ) {
                        addInterval(foregroundSince, ts)
                    }
                    foregroundPkg = ev.packageName
                    foregroundSince = ts
                }

                UsageEvents.Event.ACTIVITY_PAUSED, UsageEvents.Event.ACTIVITY_STOPPED -> {
                    if (foregroundPkg != null && ev.packageName == foregroundPkg && foregroundSince > 0) {
                        if (foregroundPkg?.let { it !in excluded } == true) {
                            addInterval(foregroundSince, ts)
                        }
                        foregroundPkg = null
                        foregroundSince = 0
                    }
                }
            }
        }
        // 区间结束时仍在前台：结算到当前
        if (foregroundPkg != null && foregroundSince > 0 &&
            foregroundPkg?.let { it !in excluded } == true
        ) {
            addInterval(foregroundSince, windowTo)
        }

        return List(n) { i ->
            DayUsage(
                date = dateString(n - 1 - i),
                hourlyMinutes = IntArray(24) { h -> (ms[i][h] / 60_000L).toInt().coerceAtLeast(0) },
                eventCount = evCount[i],
            )
        }
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

    /** 只排除桌面（2026-11-07 用户定稿）：其余一律计入。 */
    private fun excludedPackages(): Set<String> = buildSet {
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
