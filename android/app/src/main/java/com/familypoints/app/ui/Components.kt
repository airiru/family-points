package com.familypoints.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.familypoints.app.data.api.MemberBrief
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.GreenSoft
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.RedSoft
import com.familypoints.app.ui.theme.Sub

/** 统一卡片容器。 */
@Composable
fun CardBox(
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(16.dp),
        color = MaterialTheme.colorScheme.surface,
        tonalElevation = 0.dp,
        shadowElevation = 1.dp,
    ) {
        Column(Modifier.padding(14.dp)) { content() }
    }
}

/** 积分文本：正绿负红零灰。 */
@Composable
fun PointsText(points: Int, big: Boolean = false) {
    val color = when {
        points > 0 -> Green
        points < 0 -> Red
        else -> Sub
    }
    Text(
        text = if (points == 0) "—" else if (points > 0) "+$points" else "$points",
        color = color,
        fontWeight = FontWeight.Bold,
        fontSize = if (big) 22.sp else 15.sp,
    )
}

/** 头像：后端只存预设标识（emoji 或英文名），这里做映射兜底。 */
@Composable
fun Avatar(text: String?, size: Int = 42) {
    val glyph = avatarGlyph(text)
    Box(
        modifier = Modifier
            .size(size.dp)
            .clip(RoundedCornerShape((size * 0.32f).dp))
            .background(MaterialTheme.colorScheme.surfaceVariant),
        contentAlignment = Alignment.Center,
    ) {
        Text(glyph, fontSize = (size * 0.48f).sp)
    }
}

fun avatarGlyph(text: String?): String {
    if (text.isNullOrBlank()) return "🙂"
    // 已经是 emoji 就直接用
    if (text.codePoints().anyMatch { it > 0x2190 }) return text
    return when (text.lowercase()) {
        "bear" -> "🐻"
        "rabbit" -> "🐰"
        "fox" -> "🦊"
        "panda" -> "🐼"
        "tiger" -> "🐯"
        "koala" -> "🐨"
        "lion" -> "🦁"
        "frog" -> "🐸"
        else -> "🙂"
    }
}

/** 小标签。 */
@Composable
fun Pill(text: String, color: Color = Sub, bg: Color = Line) {
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(7.dp))
            .background(bg)
            .padding(horizontal = 8.dp, vertical = 3.dp)
    ) {
        Text(text, color = color, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun StatusPill(status: String) {
    when (status) {
        "open" -> Pill("可接取", Primary, PrimarySoft)
        "taken" -> Pill("进行中", Blue, Color(0xFFEAF1FF))
        "submitted" -> Pill("待投票", Primary, PrimarySoft)
        "approved" -> Pill("已通过", Green, GreenSoft)
        "rejected" -> Pill("已驳回", Red, RedSoft)
        "closed" -> Pill("已完成", Green, GreenSoft)
        "expired" -> Pill("已超时", Sub, Line)
        else -> Pill(status)
    }
}

/** 成员行。 */
@Composable
fun MemberRow(
    member: MemberBrief,
    isMe: Boolean,
    trailing: @Composable (() -> Unit)? = null,
    onClick: (() -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable { onClick() } else Modifier)
            .padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Avatar(member.avatar)
        Spacer(Modifier.width(11.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    member.name,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 15.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (isMe) {
                    Spacer(Modifier.width(6.dp))
                    Pill("我", Primary, PrimarySoft)
                }
                if (member.isLeader) {
                    Spacer(Modifier.width(6.dp))
                    Pill("组长", Primary, PrimarySoft)
                }
            }
            Text(
                buildString {
                    append("可用 ${member.score}")
                    if (member.frozenScore > 0) append(" · 冻结 ${member.frozenScore}")
                },
                fontSize = 12.sp,
                color = Sub,
            )
        }
        if (trailing != null) trailing() else PointsText(member.totalScore)
    }
}

/** 空状态。 */
@Composable
fun EmptyState(text: String) {
    Box(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 40.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = Sub, fontSize = 13.sp)
    }
}

/** 页面标题栏。 */
@Composable
fun ScreenHeader(
    title: String,
    onBack: (() -> Unit)? = null,
    action: @Composable (() -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            Surface(
                shape = RoundedCornerShape(11.dp),
                color = MaterialTheme.colorScheme.surface,
                shadowElevation = 1.dp,
                modifier = Modifier
                    .size(34.dp)
                    .clickable { onBack() },
            ) {
                Box(contentAlignment = Alignment.Center) {
                    Text("‹", fontSize = 22.sp, fontWeight = FontWeight.Bold)
                }
            }
            Spacer(Modifier.width(10.dp))
        }
        Text(title, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
        action?.invoke()
    }
}

@Composable
fun ConfirmDialog(
    title: String,
    text: String,
    confirmText: String = "确定",
    danger: Boolean = false,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title, style = MaterialTheme.typography.titleMedium) },
        text = { Text(text, fontSize = 14.sp, color = Sub) },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(confirmText, color = if (danger) Red else Primary, fontWeight = FontWeight.SemiBold)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("取消", color = Sub) }
        },
    )
}

/** 全屏加载遮罩。 */
@Composable
fun BusyOverlay(visible: Boolean) {
    if (!visible) return
    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.18f)),
        contentAlignment = Alignment.Center,
    ) {
        Surface(shape = CircleShape, color = MaterialTheme.colorScheme.surface, shadowElevation = 4.dp) {
            Box(Modifier.padding(18.dp)) {
                CircularProgressIndicator(strokeWidth = 2.5.dp, modifier = Modifier.size(26.dp), color = Primary)
            }
        }
    }
}

/** 两栏分段控件。 */
@Composable
fun SegmentedRow(
    options: List<Pair<String, Boolean>>,
    onSelect: (Int) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 6.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .padding(3.dp),
        horizontalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        options.forEachIndexed { index, (label, selected) ->
            Box(
                modifier = Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(9.dp))
                    .background(if (selected) MaterialTheme.colorScheme.surface else Color.Transparent)
                    .clickable { onSelect(index) }
                    .padding(vertical = 9.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    label,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = if (selected) MaterialTheme.colorScheme.onSurface else Sub,
                )
            }
        }
    }
}

/** 简单进度条。 */
@Composable
fun ThinProgress(current: Int, total: Int, color: Color = Green) {
    val ratio = if (total <= 0) 0f else (current.toFloat() / total).coerceIn(0f, 1f)
    Box(
        Modifier
            .fillMaxWidth()
            .height(7.dp)
            .clip(RoundedCornerShape(99.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
    ) {
        Box(
            Modifier
                .fillMaxWidth(ratio)
                .height(7.dp)
                .clip(RoundedCornerShape(99.dp))
                .background(color)
        )
    }
}
