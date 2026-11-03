package com.familypoints.app.ui.screens

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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Divider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import com.familypoints.app.data.api.TaskDetailView
import com.familypoints.app.data.api.TaskView
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.CardBox
import com.familypoints.app.ui.EmptyState
import com.familypoints.app.ui.Pill
import com.familypoints.app.ui.ScreenHeader
import com.familypoints.app.ui.SegmentedRow
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.StatusPill
import com.familypoints.app.ui.ThinProgress
import com.familypoints.app.ui.nav.Routes
import com.familypoints.app.ui.theme.Blue
import com.familypoints.app.ui.theme.Green
import com.familypoints.app.ui.theme.Line
import com.familypoints.app.ui.theme.Primary
import com.familypoints.app.ui.theme.PrimarySoft
import com.familypoints.app.ui.theme.Red
import com.familypoints.app.ui.theme.Sub

@Composable
fun TaskListScreen(vm: AppViewModel, nav: NavHostController) {
    val tasks by vm.tasks.collectAsStateWithLifecycle()
    val members by vm.members.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    var filter by remember { mutableStateOf(0) }
    var showCreate by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        vm.loadTasks()
        vm.loadOverview()
    }

    val visible = when (filter) {
        0 -> tasks.filter { it.publisherId != me?.id && it.status == "open" }
        1 -> tasks.filter { it.publisherId == me?.id }
        2 -> tasks.filter { it.status == "submitted" }
        else -> tasks
    }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(
            title = "任务",
            action = {
                Button(
                    onClick = { showCreate = true },
                    colors = ButtonDefaults.buttonColors(containerColor = Primary),
                    modifier = Modifier.height(36.dp),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp),
                ) { Text("发布", fontSize = 13.sp) }
            },
        )

        SegmentedRow(
            options = listOf(
                "可接取" to (filter == 0),
                "我发布" to (filter == 1),
                "待投票" to (filter == 2),
                "全部" to (filter == 3),
            ),
        ) { filter = it }

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (visible.isEmpty()) {
                item { EmptyState("这里还没有任务") }
            }
            items(visible, key = { it.id }) { task ->
                TaskCard(
                    task = task,
                    publisherName = members.firstOrNull { it.id == task.publisherId }?.name ?: "成员",
                    assigneeName = task.assigneeId?.let { id ->
                        members.firstOrNull { it.id == id }?.name
                    },
                    onClick = { nav.navigate(Routes.taskDetail(task.id)) },
                )
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (showCreate) {
        CreateTaskDialog(
            onDismiss = { showCreate = false },
            onConfirm = { title, points ->
                vm.createTask(title, points)
                showCreate = false
            },
        )
    }
}

@Composable
private fun TaskCard(
    task: TaskView,
    publisherName: String,
    assigneeName: String?,
    onClick: () -> Unit,
) {
    val accent = when (task.status) {
        "open" -> Primary
        "taken" -> Blue
        "submitted" -> Primary
        "closed" -> Green
        else -> Line
    }
    CardBox(modifier = Modifier.clickable { onClick() }) {
        Row(verticalAlignment = Alignment.Top) {
            Box(
                Modifier
                    .width(3.dp)
                    .height(44.dp)
                    .clip(RoundedCornerShape(2.dp))
                    .background(accent)
            )
            Spacer(Modifier.width(11.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    task.title,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 15.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    buildString {
                        append("$publisherName 发布")
                        if (assigneeName != null) append(" · $assigneeName 接取")
                    },
                    fontSize = 12.sp,
                    color = Sub,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
            Column(horizontalAlignment = Alignment.End) {
                Text("+${task.points}", color = Green, fontWeight = FontWeight.Bold, fontSize = 16.sp)
                Spacer(Modifier.height(4.dp))
                StatusPill(task.status)
            }
        }
    }
}

@Composable
private fun CreateTaskDialog(onDismiss: () -> Unit, onConfirm: (String, Int) -> Unit) {
    var title by remember { mutableStateOf("") }
    var points by remember { mutableStateOf("5") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("发布任务", style = MaterialTheme.typography.titleMedium) },
        text = {
            Column {
                OutlinedTextField(
                    value = title,
                    onValueChange = { title = it },
                    label = { Text("任务内容") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = points,
                    onValueChange = { points = it.filter { c -> c.isDigit() } },
                    label = { Text("悬赏分值（从你的积分冻结）") },
                    singleLine = true,
                    keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                        keyboardType = KeyboardType.Number
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "发布后除你之外任何成员都可接取。通过后分值转给完成者。",
                    fontSize = 11.5.sp,
                    color = Sub,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(title, points.toIntOrNull() ?: 0) },
                enabled = title.isNotBlank() && (points.toIntOrNull() ?: 0) > 0,
            ) { Text("发布", color = Primary, fontWeight = FontWeight.SemiBold) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消", color = Sub) } },
    )
}

@Composable
fun TaskDetailScreen(vm: AppViewModel, nav: NavHostController, taskId: Long) {
    val detail by vm.taskDetail.collectAsStateWithLifecycle()
    val members by vm.members.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    var showSubmit by remember { mutableStateOf(false) }
    var confirmClose by remember { mutableStateOf(false) }

    LaunchedEffect(taskId) {
        vm.loadTaskDetail(taskId)
        vm.loadOverview()
    }

    val d: TaskDetailView = detail ?: TaskDetailView()
    val task = d.task
    val isPublisher = task.publisherId == me?.id
    val isAssignee = task.assigneeId == me?.id
    val latestSub = d.submissions.lastOrNull { it.status == "submitted" }
    val tally = latestSub?.let { d.tally[it.id.toString()] }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "任务详情", onBack = { nav.popBackStack() })

        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            item {
                CardBox {
                    Text(task.title, style = MaterialTheme.typography.titleLarge)
                    Text(
                        buildString {
                            append(members.firstOrNull { it.id == task.publisherId }?.name ?: "成员")
                            append(" 发布")
                            if (task.status == "taken" || task.status == "submitted" || task.status == "closed") {
                                val an = members.firstOrNull { it.id == task.assigneeId }?.name
                                if (an != null) append(" · $an 接取")
                            }
                        },
                        fontSize = 12.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                    Spacer(Modifier.height(14.dp))
                    Row {
                        Column(Modifier.weight(1f)) {
                            Text("悬赏（已冻结）", fontSize = 12.sp, color = Sub)
                            Text(
                                "+${task.points}",
                                color = Green,
                                fontWeight = FontWeight.Bold,
                                fontSize = 22.sp,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text("状态", fontSize = 12.sp, color = Sub)
                            Spacer(Modifier.height(4.dp))
                            StatusPill(task.status)
                        }
                    }
                }
            }

            if (latestSub != null && tally != null) {
                item {
                    CardBox {
                        Text("投票进度", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                        Spacer(Modifier.height(10.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                "同意 ${tally.approve} / 需要 ${tally.need}",
                                fontSize = 12.sp,
                                color = Sub,
                                modifier = Modifier.weight(1f),
                            )
                            Text("反对 ${tally.reject}", fontSize = 12.sp, color = Sub)
                        }
                        Spacer(Modifier.height(8.dp))
                        ThinProgress(tally.approve, tally.need)
                        Text(
                            "除发布者外全员过半同意才通过；投票后不可更改。",
                            fontSize = 11.5.sp,
                            color = Sub,
                            modifier = Modifier.padding(top = 8.dp),
                        )
                        if (latestSub.note.isNotBlank()) {
                            Spacer(Modifier.height(10.dp))
                            Text("完成说明", fontSize = 12.sp, color = Sub)
                            Text(latestSub.note, fontSize = 14.sp, modifier = Modifier.padding(top = 2.dp))
                        }
                    }
                }
            }

            item {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    when {
                        task.status == "open" && !isPublisher -> {
                            Button(
                                onClick = { vm.claimTask(task.id) },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Primary),
                            ) { Text("接取任务") }
                        }

                        task.status == "taken" && isAssignee -> {
                            Button(
                                onClick = { showSubmit = true },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Primary),
                            ) { Text("提交完成") }
                            OutlinedButton(
                                onClick = { vm.abandonTask(task.id) },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp),
                            ) { Text("放弃接取", color = Sub) }
                        }

                        task.status == "submitted" && !isPublisher && latestSub != null -> {
                            Button(
                                onClick = { vm.voteTask(latestSub.id, "approve", task.id) },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp),
                                colors = ButtonDefaults.buttonColors(containerColor = Green),
                            ) { Text("投同意票") }
                            OutlinedButton(
                                onClick = { vm.voteTask(latestSub.id, "reject", task.id) },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .height(46.dp),
                            ) { Text("投反对票", color = Red) }
                        }
                    }

                    if (isPublisher && task.status != "closed") {
                        OutlinedButton(
                            onClick = { confirmClose = true },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(46.dp),
                        ) { Text("关闭任务（退回冻结积分）", color = Red) }
                    }

                    if (task.status == "open" && isPublisher) {
                        Text(
                            "这是你发布的任务，不能自己接取。",
                            fontSize = 12.sp,
                            color = Sub,
                            modifier = Modifier.fillMaxWidth(),
                            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                        )
                    }
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }

    if (showSubmit && latestSub == null) {
        SubmitDialog(
            onDismiss = { showSubmit = false },
            onConfirm = { note ->
                vm.submitTask(task.id, note)
                showSubmit = false
            },
        )
    }
    if (confirmClose) {
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirmClose = false },
            title = { Text("关闭任务？") },
            text = { Text("冻结的 ${task.points} 分会退回你的可用余额。", fontSize = 14.sp, color = Sub) },
            confirmButton = {
                TextButton(onClick = {
                    vm.closeTask(task.id)
                    confirmClose = false
                }) { Text("关闭", color = Red, fontWeight = FontWeight.SemiBold) }
            },
            dismissButton = { TextButton(onClick = { confirmClose = false }) { Text("取消", color = Sub) } },
        )
    }
}

@Composable
private fun SubmitDialog(onDismiss: () -> Unit, onConfirm: (String) -> Unit) {
    var note by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("提交完成") },
        text = {
            Column {
                OutlinedTextField(
                    value = note,
                    onValueChange = { note = it },
                    label = { Text("说明你做了什么（必填）") },
                    minLines = 3,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "提交后由其他成员投票决定是否通过，过半同意才会转移积分。",
                    fontSize = 11.5.sp,
                    color = Sub,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(note) }, enabled = note.isNotBlank()) {
                Text("提交", color = Primary, fontWeight = FontWeight.SemiBold)
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消", color = Sub) } },
    )
}

@Composable
fun VoteScreen(vm: AppViewModel, nav: NavHostController) {
    val tasks by vm.tasks.collectAsStateWithLifecycle()
    val session by vm.session.collectAsStateWithLifecycle()
    val me = (session as? Session.Ready)?.me

    LaunchedEffect(Unit) { vm.loadTasks() }

    val pending = tasks.filter { it.status == "submitted" && it.publisherId != me?.id }

    Column(Modifier.fillMaxSize()) {
        ScreenHeader(title = "待投票", onBack = { nav.popBackStack() })
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            if (pending.isEmpty()) {
                item { EmptyState("暂无待投票任务") }
            }
            items(pending, key = { it.id }) { task ->
                CardBox(modifier = Modifier.clickable { nav.navigate(Routes.taskDetail(task.id)) }) {
                    Text(task.title, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                    Text(
                        "悬赏 +${task.points} · 点进去查看详情并投票",
                        fontSize = 12.sp,
                        color = Sub,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                }
            }
            item { Spacer(Modifier.height(12.dp)) }
        }
    }
}
