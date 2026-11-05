package com.familypoints.app.util

import android.annotation.SuppressLint
import android.content.Context
import android.os.Build
import android.provider.Settings

/**
 * 设备标识：同一账号多设备时用于区分上报来源。
 *
 * 用 ANDROID_ID 而不是 IMEI/序列号：后者在 Android 10+ 已不可读，且属于隐私敏感信息。
 * ANDROID_ID 是「应用 × 设备 × 用户」维度的稳定值，卸载重装会变——对家庭场景够用。
 */
object DeviceInfo {

    @SuppressLint("HardwareIds")
    fun key(context: Context): String {
        val id = runCatching {
            Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID)
        }.getOrNull()
        return if (id.isNullOrBlank()) "unknown" else id.take(64)
    }

    /** 展示用型号：如 NOH-AN80 / Pixel 6。 */
    fun model(): String {
        val m = Build.MODEL ?: ""
        val b = Build.BRAND ?: ""
        val s = if (m.startsWith(b, ignoreCase = true)) m else "$b $m".trim()
        return if (s.isBlank()) "Android" else s.take(64)
    }
}
