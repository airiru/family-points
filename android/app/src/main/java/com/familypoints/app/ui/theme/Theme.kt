package com.familypoints.app.ui.theme

import android.app.Activity
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat

val Primary = Color(0xFFFF7A3D)
val PrimarySoft = Color(0xFFFFF1E9)
val Green = Color(0xFF16A34A)
val GreenSoft = Color(0xFFE8F7EE)
val Red = Color(0xFFE5484D)
val RedSoft = Color(0xFFFDECEC)
val Blue = Color(0xFF3B82F6)
val Ink = Color(0xFF141821)
val Sub = Color(0xFF6B7280)
val Line = Color(0xFFEEF0F5)
val AppBg = Color(0xFFF6F7FB)
val CardBg = Color(0xFFFFFFFF)

private val LightColors = lightColorScheme(
    primary = Primary,
    onPrimary = Color.White,
    primaryContainer = PrimarySoft,
    onPrimaryContainer = Primary,
    secondary = Blue,
    background = AppBg,
    onBackground = Ink,
    surface = CardBg,
    onSurface = Ink,
    surfaceVariant = Line,
    onSurfaceVariant = Sub,
    error = Red,
    onError = Color.White,
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFFF9F5A),
    onPrimary = Color(0xFF3A1B08),
    primaryContainer = Color(0xFF4A2410),
    onPrimaryContainer = Color(0xFFFFD9C2),
    background = Color(0xFF12141A),
    onBackground = Color(0xFFE8EAF0),
    surface = Color(0xFF1B1E26),
    onSurface = Color(0xFFE8EAF0),
    surfaceVariant = Color(0xFF2A2E38),
    onSurfaceVariant = Color(0xFFA8AEBB),
    error = Color(0xFFFF6B6B),
)

private val AppTypography = Typography(
    headlineMedium = TextStyle(fontSize = 24.sp, fontWeight = FontWeight.Bold, letterSpacing = (-0.4).sp),
    titleLarge = TextStyle(fontSize = 19.sp, fontWeight = FontWeight.Bold, letterSpacing = (-0.2).sp),
    titleMedium = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.SemiBold),
    bodyLarge = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.Normal),
    bodyMedium = TextStyle(fontSize = 14.sp, fontWeight = FontWeight.Normal),
    bodySmall = TextStyle(fontSize = 12.sp, fontWeight = FontWeight.Normal),
    labelSmall = TextStyle(fontSize = 11.sp, fontWeight = FontWeight.Medium),
)

@Composable
fun FamilyPointsTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val colors = if (darkTheme) DarkColors else LightColors
    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as Activity).window
            window.statusBarColor = Color.Transparent.toArgb()
            WindowCompat.getInsetsController(window, view).isAppearanceLightStatusBars = !darkTheme
        }
    }
    MaterialTheme(colorScheme = colors, typography = AppTypography, content = content)
}
