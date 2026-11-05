package com.familypoints.app.data.local

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.dataStore by preferencesDataStore(name = "fp_auth")

/**
 * 登录凭证与设备令牌的本地存储。
 *
 * access token 有效期短（2h），refresh token 长（30d）——
 * 两者都持久化，冷启动时先用 access，过期由拦截器自动换新。
 */
@Singleton
class TokenStore @Inject constructor(
    @ApplicationContext private val context: Context,
) {

    private object Keys {
        val ACCESS = stringPreferencesKey("access_token")
        val REFRESH = stringPreferencesKey("refresh_token")
        val MEMBER_ID = stringPreferencesKey("member_id")
        val MEMBER_NAME = stringPreferencesKey("member_name")
        val ROLE = stringPreferencesKey("role")
        val FAMILY_ID = stringPreferencesKey("family_id")
    }

    val accessToken: Flow<String?> = context.dataStore.data.map { it[Keys.ACCESS] }
    val refreshToken: Flow<String?> = context.dataStore.data.map { it[Keys.REFRESH] }
    val memberId: Flow<Long?> = context.dataStore.data.map { it[Keys.MEMBER_ID]?.toLongOrNull() }
    val role: Flow<String?> = context.dataStore.data.map { it[Keys.ROLE] }
    val familyId: Flow<Long?> = context.dataStore.data.map { it[Keys.FAMILY_ID]?.toLongOrNull() }

    suspend fun accessTokenOnce(): String? = accessToken.first()
    suspend fun refreshTokenOnce(): String? = refreshToken.first()
    suspend fun memberIdOnce(): Long? = memberId.first()

    suspend fun saveSession(
        access: String,
        refresh: String,
        memberId: Long,
        name: String,
        role: String,
        familyId: Long?,
    ) {
        context.dataStore.edit {
            it[Keys.ACCESS] = access
            it[Keys.REFRESH] = refresh
            it[Keys.MEMBER_ID] = memberId.toString()
            it[Keys.MEMBER_NAME] = name
            it[Keys.ROLE] = role
            if (familyId != null) it[Keys.FAMILY_ID] = familyId.toString() else it.remove(Keys.FAMILY_ID)
        }
    }

    suspend fun updateTokens(access: String, refresh: String) {
        context.dataStore.edit {
            it[Keys.ACCESS] = access
            it[Keys.REFRESH] = refresh
        }
    }

    suspend fun updateRole(role: String) {
        context.dataStore.edit { it[Keys.ROLE] = role }
    }

    suspend fun clear() {
        // 只清登录态，保留设备令牌：设备令牌是管理员签发的，重新登录不该丢
        context.dataStore.edit {
            it.remove(Keys.ACCESS)
            it.remove(Keys.REFRESH)
            it.remove(Keys.MEMBER_ID)
            it.remove(Keys.MEMBER_NAME)
            it.remove(Keys.ROLE)
            it.remove(Keys.FAMILY_ID)
        }
    }
}
