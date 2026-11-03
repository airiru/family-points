package com.familypoints.app.ui.nav

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.Checklist
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Storefront
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.familypoints.app.ui.AppViewModel
import com.familypoints.app.ui.Session
import com.familypoints.app.ui.screens.BoardScreen
import com.familypoints.app.ui.screens.CouponsScreen
import com.familypoints.app.ui.screens.GiftScreen
import com.familypoints.app.ui.screens.GoodsScreen
import com.familypoints.app.ui.screens.JoinFamilyScreen
import com.familypoints.app.ui.screens.LoginScreen
import com.familypoints.app.ui.screens.ManageMembersScreen
import com.familypoints.app.ui.screens.MemberDetailScreen
import com.familypoints.app.ui.screens.NotificationsScreen
import com.familypoints.app.ui.screens.ProfileScreen
import com.familypoints.app.ui.screens.RecordsScreen
import com.familypoints.app.ui.screens.RegisterScreen
import com.familypoints.app.ui.screens.ScreenTimeScreen
import com.familypoints.app.ui.screens.ShopScreen
import com.familypoints.app.ui.screens.SleepRuleScreen
import com.familypoints.app.ui.screens.TaskDetailScreen
import com.familypoints.app.ui.screens.TaskListScreen
import com.familypoints.app.ui.screens.VoteScreen

object Routes {
    const val LOGIN = "login"
    const val REGISTER = "register"
    const val JOIN = "join"

    const val BOARD = "board"
    const val TASK = "task"
    const val SHOP = "shop"
    const val NOTIFY = "notify"
    const val ME = "me"

    const val MEMBER_DETAIL = "member/{id}"
    const val RECORDS = "records"
    const val GIFT = "gift"
    const val TASK_DETAIL = "taskDetail/{id}"
    const val VOTE = "vote"
    const val SCREEN_TIME = "screenTime"
    const val MANAGE_MEMBERS = "manageMembers"
    const val SLEEP_RULE = "sleepRule"
    const val GOODS = "goods"
    const val COUPONS = "coupons"

    fun memberDetail(id: Long) = "member/$id"
    fun taskDetail(id: Long) = "taskDetail/$id"
}

private data class TabItem(val route: String, val label: String, val icon: ImageVector)

private val tabs = listOf(
    TabItem(Routes.BOARD, "积分榜", Icons.Filled.BarChart),
    TabItem(Routes.TASK, "任务", Icons.Filled.Checklist),
    TabItem(Routes.SHOP, "商城", Icons.Filled.Storefront),
    TabItem(Routes.NOTIFY, "通知", Icons.Filled.Notifications),
    TabItem(Routes.ME, "我的", Icons.Filled.Person),
)

@Composable
fun AppNav(vm: AppViewModel) {
    val session by vm.session.collectAsStateWithLifecycle()
    when (val s = session) {
        Session.Loading -> SplashScreen()
        Session.LoggedOut -> AuthNav(vm)
        is Session.NoFamily -> JoinFamilyScreen(vm, s.me)
        is Session.Ready -> MainNav(vm)
    }
}

@Composable
private fun SplashScreen() {
    Box(Modifier.fillMaxSize(), contentAlignment = androidx.compose.ui.Alignment.Center) {
        Text("家庭积分", fontSize = 20.sp, fontWeight = FontWeight.Bold)
    }
}

@Composable
private fun AuthNav(vm: AppViewModel) {
    val nav = rememberNavController()
    NavHost(navController = nav, startDestination = Routes.LOGIN) {
        composable(Routes.LOGIN) {
            LoginScreen(
                vm = vm,
                onGoRegister = { nav.navigate(Routes.REGISTER) },
            )
        }
        composable(Routes.REGISTER) {
            RegisterScreen(vm = vm, onBack = { nav.popBackStack() })
        }
    }
}

@Composable
private fun MainNav(vm: AppViewModel) {
    val nav = rememberNavController()
    val backStack by nav.currentBackStackEntryAsState()
    val current = backStack?.destination?.route
    val showBar = tabs.any { it.route == current }

    Scaffold(
        bottomBar = {
            if (showBar) {
                BottomBar(nav, current, vm)
            }
        }
    ) { padding ->
        NavHost(
            navController = nav,
            startDestination = Routes.BOARD,
            modifier = Modifier.padding(padding),
        ) {
            composable(Routes.BOARD) { BoardScreen(vm, nav) }
            composable(Routes.TASK) { TaskListScreen(vm, nav) }
            composable(Routes.SHOP) { ShopScreen(vm, nav) }
            composable(Routes.NOTIFY) { NotificationsScreen(vm, nav) }
            composable(Routes.ME) { ProfileScreen(vm, nav) }

            composable(
                Routes.MEMBER_DETAIL,
                arguments = listOf(navArgument("id") { type = NavType.LongType }),
            ) { entry ->
                MemberDetailScreen(vm, nav, entry.arguments?.getLong("id") ?: 0L)
            }
            composable(Routes.RECORDS) { RecordsScreen(vm, nav) }
            composable(Routes.GIFT) { GiftScreen(vm, nav) }
            composable(
                Routes.TASK_DETAIL,
                arguments = listOf(navArgument("id") { type = NavType.LongType }),
            ) { entry ->
                TaskDetailScreen(vm, nav, entry.arguments?.getLong("id") ?: 0L)
            }
            composable(Routes.VOTE) { VoteScreen(vm, nav) }
            composable(Routes.SCREEN_TIME) { ScreenTimeScreen(vm, nav) }
            composable(Routes.MANAGE_MEMBERS) { ManageMembersScreen(vm, nav) }
            composable(Routes.SLEEP_RULE) { SleepRuleScreen(vm, nav) }
            composable(Routes.GOODS) { GoodsScreen(vm, nav) }
            composable(Routes.COUPONS) { CouponsScreen(vm, nav) }
        }
    }
}

@Composable
private fun BottomBar(nav: NavHostController, current: String?, vm: AppViewModel) {
    val unread by vm.unread.collectAsStateWithLifecycle()
    NavigationBar(containerColor = androidx.compose.material3.MaterialTheme.colorScheme.surface) {
        tabs.forEach { tab ->
            NavigationBarItem(
                selected = current == tab.route,
                onClick = {
                    if (current != tab.route) {
                        nav.navigate(tab.route) {
                            popUpTo(nav.graph.findStartDestination().id) { saveState = true }
                            launchSingleTop = true
                            restoreState = true
                        }
                    }
                },
                icon = {
                    Box {
                        Icon(tab.icon, contentDescription = tab.label)
                        if (tab.route == Routes.NOTIFY && unread > 0) {
                            androidx.compose.material3.Badge(
                                modifier = Modifier
                                    .align(androidx.compose.ui.Alignment.TopEnd)
                                    .padding(start = 12.dp)
                            ) { Text(if (unread > 99) "99+" else "$unread", fontSize = 9.sp) }
                        }
                    }
                },
                label = { Text(tab.label, fontSize = 10.5.sp, fontWeight = FontWeight.SemiBold) },
            )
        }
    }
}

