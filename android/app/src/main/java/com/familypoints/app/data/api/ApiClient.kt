package com.familypoints.app.data.api

import com.familypoints.app.BuildConfig
import com.familypoints.app.data.local.TokenStore
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.Authenticator
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okhttp3.Route
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton

val AppJson = Json {
    ignoreUnknownKeys = true   // 后端加字段不该让客户端崩溃
    explicitNulls = false
    coerceInputValues = true
}

/**
 * OkHttp 客户端。
 *
 * 两个职责分开：
 * - [AuthInterceptor]：给每个 /api/app 请求带 Bearer；顺便记录 401 次数
 * - [TokenAuthenticator]：收到 401 时用 refresh token 换新的 access，并重放原请求
 *
 * 不用「拦截器里判断 401 后刷新」，因为 Authenticator 是 OkHttp 官方的重放机制，
 * 能正确处理并发请求只刷新一次的问题（这里用 synchronized 再兜一层）。
 */
@Singleton
class ApiClient @Inject constructor(
    private val tokenStore: TokenStore,
) {
    private val json = AppJson

    // 会话失效事件（2026-11-07）：refresh 也失败 = 账号被停用/注销/令牌被吊销，
    // App 必须回登录页而不是停在原地弹 401。VM 订阅它做本地登出。
    private val _sessionInvalid = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val sessionInvalid: SharedFlow<Unit> = _sessionInvalid

    private val authInterceptor = Interceptor { chain ->
        val original = chain.request()
        val path = original.url.encodedPath
        // 公开接口与设备上报不需要 JWT
        val needAuth = path.startsWith("/api/app") || path.startsWith("/api/admin")
        if (!needAuth) return@Interceptor chain.proceed(original)

        val token = runBlocking { tokenStore.accessTokenOnce() }
        val req = if (token.isNullOrBlank()) original else original.newBuilder()
            .header("Authorization", "Bearer $token")
            .build()
        chain.proceed(req)
    }

    private val authenticator = object : Authenticator {
        private val lock = Any()

        override fun authenticate(route: Route?, response: Response): Request? {
            // 防死循环：已经重试过的请求不再刷新
            if (response.request.header("X-Retry-Auth") != null) return null

            synchronized(lock) {
                val reqToken = response.request.header("Authorization")?.removePrefix("Bearer ")?.trim()
                val current = runBlocking { tokenStore.accessTokenOnce() }
                // 别的线程已经刷新过了，直接用新 token 重放
                if (!current.isNullOrBlank() && current != reqToken) {
                    return response.request.newBuilder()
                        .header("Authorization", "Bearer $current")
                        .header("X-Retry-Auth", "1")
                        .build()
                }
                val refresh = runBlocking { tokenStore.refreshTokenOnce() }
                if (refresh.isNullOrBlank()) {
                    runBlocking { tokenStore.clear() }
                    _sessionInvalid.tryEmit(Unit)
                    return null
                }
                val newPair = runBlocking { refreshBlocking(refresh) }
                if (newPair == null) {
                    // refresh 也被拒（停用 / 注销 / 吊销）：本地会话已死，通知 VM 回登录页
                    runBlocking { tokenStore.clear() }
                    _sessionInvalid.tryEmit(Unit)
                    return null
                }
                runBlocking { tokenStore.updateTokens(newPair.accessToken, newPair.refreshToken) }
                return response.request.newBuilder()
                    .header("Authorization", "Bearer ${newPair.accessToken}")
                    .header("X-Retry-Auth", "1")
                    .build()
            }
        }
    }

    /** Authenticator 在 OkHttp 线程里同步执行，这里用独立 client 避免递归。 */
    private fun refreshBlocking(refreshToken: String): TokenPair? = runCatching {
        val plain = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(15, TimeUnit.SECONDS)
            .build()
        val body = AppJson.encodeToString(RefreshReq.serializer(), RefreshReq(refreshToken))
        val req = Request.Builder()
            .url(BuildConfig.API_BASE_URL + "api/app/refresh")
            .post(
                okhttp3.RequestBody.create(
                    "application/json; charset=utf-8".toMediaType(),
                    body,
                )
            )
            .build()
        plain.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return null
            val text = resp.body?.string() ?: return null
            AppJson.decodeFromString(Envelope.serializer(AuthResult.serializer()), text)
                .data?.token
        }
    }.getOrNull()

    /**
     * 把「业务错误」的 HTTP 状态改写成 200。
     *
     * 后端对业务失败返回 `{ok:false,error:"组长需先转让…"}` 配 4xx，
     * 而 Retrofit 遇到非 2xx 直接抛 HttpException，body 被丢掉，
     * 用户只看到「HTTP 400」——正是 2026-11-06 反馈的问题。
     *
     * 只改写 400/403/404/409/422：
     * - **不动 401**（TokenAuthenticator 靠它触发刷新）
     * - **不动 5xx**（那是真故障，应该显眼）
     * 改写后 `Envelope.ok=false` 且 `error` 有值，各页面照常显示中文提示。
     */
    private val businessErrorInterceptor = Interceptor { chain ->
        val resp = chain.proceed(chain.request())
        val code = resp.code
        val soft = code == 400 || code == 403 || code == 404 || code == 409 || code == 422
        if (!soft) return@Interceptor resp

        val body = resp.body
        val text = body?.string()
        if (text != null && text.contains("\"ok\":false")) {
            return@Interceptor resp.newBuilder()
                .code(200)
                .message("OK")
                .body(text.toResponseBody("application/json; charset=utf-8".toMediaType()))
                .build()
        }
        // 没命中业务错误格式：把 body 原样放回，不改变行为
        resp.newBuilder().body(text?.toResponseBody(body?.contentType())).build()
    }

    private val okHttp = OkHttpClient.Builder()
        .addInterceptor(authInterceptor)
        .addInterceptor(
            HttpLoggingInterceptor().apply {
                level = if (BuildConfig.DEBUG) HttpLoggingInterceptor.Level.BASIC
                else HttpLoggingInterceptor.Level.NONE
            }
        )
        .addInterceptor(businessErrorInterceptor)
        .authenticator(authenticator)
        .connectTimeout(20, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build()

    val service: ApiService = Retrofit.Builder()
        .baseUrl(BuildConfig.API_BASE_URL)
        .client(okHttp)
        .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
        .build()
        .create(ApiService::class.java)
}
