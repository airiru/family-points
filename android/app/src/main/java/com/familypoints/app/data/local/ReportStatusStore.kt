package com.familypoints.app.data.local

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.reportStatusStore by preferencesDataStore(name = "fp_report_status")

/** 上报结果的本地快照（供「上报状态」卡片展示真实情况）。 */
data class ReportStatus(
    val lastSuccessAt: Long?,
    val lastFailAt: Long?,
    val lastError: String?,
)

/**
 * 屏幕时间上报结果记录。
 *
 * Worker 每次跑完都把结果写进来——否则 UI 只能显示「已授权，按计划自动上报」这种
 * 无条件乐观的文案，ROM 杀掉定时任务 / 接口连不上时用户完全看不到异常（2026-11-07 真机实案：
 * 小机 HyperOS 把进程杀成 stopped 后 WorkManager 不再运行，页面却一直显示「正常」）。
 */
@Singleton
class ReportStatusStore @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private object Keys {
        val LAST_SUCCESS = longPreferencesKey("last_success_at")
        val LAST_FAIL = longPreferencesKey("last_fail_at")
        val LAST_ERROR = stringPreferencesKey("last_error")
    }

    /** 记录一次成功上报。 */
    suspend fun recordSuccess(at: Long) {
        context.reportStatusStore.edit { it[Keys.LAST_SUCCESS] = at }
    }

    /** 记录一次失败（含原因，会显示在「上报状态」卡片上）。 */
    suspend fun recordFailure(at: Long, error: String) {
        context.reportStatusStore.edit {
            it[Keys.LAST_FAIL] = at
            it[Keys.LAST_ERROR] = error.take(200)
        }
    }

    val status: Flow<ReportStatus> = context.reportStatusStore.data.map { p ->
        ReportStatus(
            lastSuccessAt = p[Keys.LAST_SUCCESS],
            lastFailAt = p[Keys.LAST_FAIL],
            lastError = p[Keys.LAST_ERROR],
        )
    }
}
