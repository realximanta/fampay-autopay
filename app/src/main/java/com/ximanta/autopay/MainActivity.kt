package com.ximanta.autopay

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.BitmapFactory
import android.os.Bundle
import android.util.Base64
import android.util.Log
import android.webkit.CookieManager
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.launch
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.TimeUnit

// --- THEME COLORS ---
val BgBlack = Color(0xFF000000)
val SurfaceDark = Color(0xFF0a0a0a)
val BorderGray = Color(0xFF222222)
val TextWhite = Color(0xFFededed)
val TextMuted = Color(0xFF888888)
val AccentYellow = Color(0xFFfacc15)
val AccentGreen = Color(0xFF22c55e)
val AccentRed = Color(0xFFef4444)
val AccentBlue = Color(0xFF3b82f6)

// --- NETWORK CLIENT ---
val client = OkHttpClient.Builder()
    .connectTimeout(15, TimeUnit.SECONDS)
    .readTimeout(15, TimeUnit.SECONDS)
    .build()

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                AutoPayApp()
            }
        }
    }
}

fun registerFcmTokenWithBackend(context: Context, cookie: String) {
    FirebaseMessaging.getInstance().token.addOnCompleteListener { task ->
        if (!task.isSuccessful) return@addOnCompleteListener
        val token = task.result
        val json = JSONObject().put("token", token)
        val body = json.toString().toRequestBody("application/json".toMediaType())
        val request = Request.Builder()
            .url("https://auto-payment.ximanta.xyz/api/notifications/register")
            .header("Cookie", cookie)
            .post(body)
            .build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) {}
            override fun onResponse(call: Call, response: Response) { response.close() }
        })
    }
}

@Composable
fun AutoPayApp() {
    val context = LocalContext.current
    val prefs = context.getSharedPreferences("autopay_prefs", Context.MODE_PRIVATE)
    
    var sessionCookie by remember { mutableStateOf(prefs.getString("session_cookie", null)) }
    var userEmail by remember { mutableStateOf("Loading...") }
    var showWebView by remember { mutableStateOf(false) }
    var currentScreen by remember { mutableStateOf("home") }

    // Modals
    var showQrModal by remember { mutableStateOf(false) }
    var qrBase64 by remember { mutableStateOf("") }
    var qrFingerprint by remember { mutableStateOf("") }
    
    var showNotifications by remember { mutableStateOf(false) }
    var notificationList by remember { mutableStateOf<List<AppNotification>>(emptyList()) }
    var hasUnread by remember { mutableStateOf(false) }

    LaunchedEffect(sessionCookie) {
        if (sessionCookie != null) {
            registerFcmTokenWithBackend(context, sessionCookie!!)
            
            // Fetch user email and check notifications
            val request = Request.Builder().url("https://auto-payment.ximanta.xyz/api/me").header("Cookie", sessionCookie!!).build()
            client.newCall(request).enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {}
                override fun onResponse(call: Call, response: Response) {
                    response.body?.string()?.let {
                        try { userEmail = JSONObject(it).optString("email", "Unknown") } catch (_: Exception) {}
                    }
                }
            })
            fetchNotifications(sessionCookie!!) { list ->
                notificationList = list
                hasUnread = list.any { !it.read }
            }
        }
    }

    if (showWebView) {
        OAuthWebView(
            onSuccess = { cookie ->
                prefs.edit().putString("session_cookie", cookie).apply()
                sessionCookie = cookie
                showWebView = false
            },
            onCancel = { showWebView = false }
        )
    } else {
        if (sessionCookie == null) {
            LandingScreen(onSignInClick = { showWebView = true })
        } else {
            val drawerState = rememberDrawerState(DrawerValue.Closed)
            val scope = rememberCoroutineScope()

            ModalNavigationDrawer(
                drawerState = drawerState,
                drawerContent = {
                    ModalDrawerSheet(drawerContainerColor = SurfaceDark) {
                        Spacer(Modifier.height(24.dp))
                        Text("Menu", color = TextWhite, fontSize = 20.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(16.dp))
                        NavigationDrawerItem(label = { Text("Dashboard", color = TextWhite) }, selected = currentScreen == "home", onClick = { currentScreen = "home"; scope.launch { drawerState.close() } })
                        NavigationDrawerItem(label = { Text("Documentation", color = TextWhite) }, selected = false, onClick = { scope.launch { drawerState.close() } })
                        NavigationDrawerItem(label = { Text("Logout", color = AccentRed) }, selected = false, onClick = { 
                            prefs.edit().remove("session_cookie").apply()
                            sessionCookie = null
                            scope.launch { drawerState.close() }
                        })
                    }
                }
            ) {
                Scaffold(
                    containerColor = BgBlack,
                    topBar = {
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(16.dp),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            IconButton(onClick = { scope.launch { drawerState.open() } }) {
                                Icon(Icons.Default.Menu, contentDescription = "Menu", tint = TextWhite)
                            }
                            
                            // NOTIFICATION BELL
                            Box {
                                IconButton(onClick = { 
                                    showNotifications = true
                                    markNotificationsRead(sessionCookie!!) { hasUnread = false }
                                }) {
                                    Icon(Icons.Default.Notifications, contentDescription = "Notifications", tint = TextWhite)
                                }
                                if (hasUnread) {
                                    Box(
                                        modifier = Modifier.size(10.dp).background(AccentRed, RoundedCornerShape(50))
                                            .align(Alignment.TopEnd).offset(x = (-8).dp, y = 8.dp)
                                    )
                                }
                            }
                        }
                    },
                    bottomBar = { BottomNavBar(currentScreen) { currentScreen = it } }
                ) { padding ->
                    Box(modifier = Modifier.padding(padding).fillMaxSize()) {
                        when (currentScreen) {
                            "home" -> DashboardScreen(
                                cookie = sessionCookie!!, 
                                onPlanClick = { plan ->
                                    val json = JSONObject().put("plan_id", plan)
                                    val body = json.toString().toRequestBody("application/json".toMediaType())
                                    val req = Request.Builder().url("https://auto-payment.ximanta.xyz/api/qr").header("Cookie", sessionCookie!!).post(body).build()
                                    client.newCall(req).enqueue(object : Callback {
                                        override fun onFailure(call: Call, e: IOException) {}
                                        override fun onResponse(call: Call, response: Response) {
                                            response.body?.string()?.let { resBody ->
                                                try {
                                                    val obj = JSONObject(resBody)
                                                    qrBase64 = obj.getString("qr_base64")
                                                    qrFingerprint = obj.getString("fingerprint")
                                                    showQrModal = true
                                                } catch (_: Exception) {}
                                            }
                                        }
                                    })
                                }
                            )
                            "history" -> HistoryScreen(sessionCookie!!)
                            "profile" -> ProfileScreen(userEmail, onLogout = {
                                prefs.edit().remove("session_cookie").apply()
                                sessionCookie = null
                            })
                        }
                    }
                }
            }
            
            if (showQrModal && qrBase64.isNotEmpty()) {
                QrCodeDialog(base64 = qrBase64, amount = qrFingerprint, cookie = sessionCookie!!, onDismiss = { showQrModal = false })
            }

            if (showNotifications) {
                NotificationsDialog(notifications = notificationList, onDismiss = { showNotifications = false })
            }
        }
    }
}

// --- NETWORK HELPERS FOR NOTIFICATIONS ---
fun fetchNotifications(cookie: String, onResult: (List<AppNotification>) -> Unit) {
    val request = Request.Builder().url("https://auto-payment.ximanta.xyz/api/notifications").header("Cookie", cookie).build()
    client.newCall(request).enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {}
        override fun onResponse(call: Call, response: Response) {
            response.body?.string()?.let { body ->
                try {
                    val jsonArray = JSONArray(body)
                    val list = mutableListOf<AppNotification>()
                    for (i in 0 until jsonArray.length()) {
                        val obj = jsonArray.getJSONObject(i)
                        list.add(AppNotification(obj.optString("title"), obj.optString("body"), obj.optLong("timestamp"), obj.optBoolean("read")))
                    }
                    onResult(list)
                } catch (_: Exception) {}
            }
        }
    })
}

fun markNotificationsRead(cookie: String, onDone: () -> Unit) {
    val request = Request.Builder().url("https://auto-payment.ximanta.xyz/api/notifications/read").header("Cookie", cookie).post("".toRequestBody()).build()
    client.newCall(request).enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {}
        override fun onResponse(call: Call, response: Response) { onDone() }
    })
}

data class AppNotification(val title: String, val body: String, val timestamp: Long, val read: Boolean)

// --- UI COMPONENTS ---

@Composable
fun NotificationsDialog(notifications: List<AppNotification>, onDismiss: () -> Unit) {
    Dialog(onDismissRequest = onDismiss) {
        Column(
            modifier = Modifier.background(SurfaceDark, RoundedCornerShape(16.dp)).border(1.dp, BorderGray, RoundedCornerShape(16.dp)).padding(16.dp).fillMaxWidth().heightIn(max = 500.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text("Notifications", color = TextWhite, fontSize = 20.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(bottom = 16.dp))
            
            if (notifications.isEmpty()) {
                Text("No notifications yet.", color = TextMuted)
            } else {
                LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(notifications) { notif ->
                        Column(modifier = Modifier.fillMaxWidth().background(BgBlack, RoundedCornerShape(8.dp)).padding(12.dp)) {
                            Text(notif.title, color = TextWhite, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                            Text(notif.body, color = TextMuted, fontSize = 12.sp, modifier = Modifier.padding(vertical = 4.dp))
                            val time = SimpleDateFormat("dd MMM, HH:mm", Locale.getDefault()).format(Date(notif.timestamp * 1000))
                            Text(time, color = AccentBlue, fontSize = 10.sp)
                        }
                    }
                }
            }
            
            TextButton(onClick = onDismiss, modifier = Modifier.padding(top = 8.dp)) {
                Text("Close", color = AccentRed)
            }
        }
    }
}

@Composable
fun LandingScreen(onSignInClick: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        Icon(Icons.Default.Bolt, contentDescription = null, tint = TextWhite, modifier = Modifier.size(64.dp))
        Spacer(modifier = Modifier.height(16.dp))
        Text("Auto-Pay", color = TextWhite, fontSize = 32.sp, fontWeight = FontWeight.Bold)
        Text("Automated Payment Verification", color = TextMuted, fontSize = 16.sp, modifier = Modifier.padding(top = 8.dp))
        Spacer(modifier = Modifier.height(48.dp))
        Button(
            onClick = onSignInClick,
            colors = ButtonDefaults.buttonColors(containerColor = TextWhite),
            modifier = Modifier.fillMaxWidth().height(56.dp),
            shape = RoundedCornerShape(12.dp)
        ) {
            Text("Sign In with Google", color = BgBlack, fontSize = 16.sp, fontWeight = FontWeight.Bold)
        }
    }
}

@SuppressLint("SetJavaScriptEnabled")
@Composable
fun OAuthWebView(onSuccess: (String) -> Unit, onCancel: () -> Unit) {
    Column(modifier = Modifier.fillMaxSize().background(BgBlack)) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text("Sign In", color = TextWhite, fontSize = 18.sp, fontWeight = FontWeight.Bold)
            IconButton(onClick = onCancel) {
                Icon(Icons.Default.Close, contentDescription = "Close", tint = TextWhite)
            }
        }
        
        AndroidView(
            factory = { context ->
                WebView(context).apply {
                    settings.javaScriptEnabled = true
                    settings.domStorageEnabled = true
                    // THIS FIXES THE GOOGLE OAUTH BLOCK
                    settings.userAgentString = "Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"
                    CookieManager.getInstance().setAcceptCookie(true)
                    
                    webViewClient = object : WebViewClient() {
                        override fun onPageFinished(view: WebView?, url: String?) {
                            super.onPageFinished(view, url)
                            if (url != null && url.contains("auto-payment.ximanta.xyz/dashboard")) {
                                val cookies = CookieManager.getInstance().getCookie(url)
                                val sessionCookie = cookies?.split(";")?.find { it.trim().startsWith("session=") }?.trim()
                                if (sessionCookie != null) {
                                    onSuccess(sessionCookie)
                                }
                            }
                        }
                    }
                    loadUrl("https://auto-payment.ximanta.xyz/api/login")
                }
            },
            modifier = Modifier.fillMaxSize()
        )
    }
}

@Composable
fun DashboardScreen(cookie: String, onPlanClick: (String) -> Unit) {
    var apiKey by remember { mutableStateOf("Loading...") }
    var daysLeft by remember { mutableStateOf("0") }
    val context = LocalContext.current

    LaunchedEffect(cookie) {
        val request = Request.Builder().url("https://auto-payment.ximanta.xyz/api/me").header("Cookie", cookie).build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) { apiKey = "Network Error" }
            override fun onResponse(call: Call, response: Response) {
                response.body?.string()?.let { body ->
                    try {
                        val json = JSONObject(body)
                        apiKey = json.optString("api_key", "No Key")
                        daysLeft = json.optString("days_left", "0")
                    } catch (e: Exception) { apiKey = "Parse Error" }
                }
            }
        })
    }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Column(
            modifier = Modifier.fillMaxWidth().background(SurfaceDark, RoundedCornerShape(12.dp))
                .border(1.dp, BorderGray, RoundedCornerShape(12.dp)).padding(16.dp)
        ) {
            Text("Your API Key", color = TextMuted, fontSize = 12.sp, fontWeight = FontWeight.Bold)
            Spacer(modifier = Modifier.height(8.dp))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(apiKey, color = AccentYellow, fontSize = 20.sp, fontFamily = FontFamily.Monospace)
                IconButton(onClick = {
                    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
                    clipboard.setPrimaryClip(android.content.ClipData.newPlainText("API Key", apiKey))
                }) { Icon(Icons.Default.ContentCopy, contentDescription = "Copy", tint = TextMuted) }
            }
        }

        Spacer(modifier = Modifier.height(16.dp))

        Column(
            modifier = Modifier.fillMaxWidth().background(SurfaceDark, RoundedCornerShape(12.dp))
                .border(1.dp, BorderGray, RoundedCornerShape(12.dp)).padding(16.dp)
        ) {
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Column {
                    Text("Subscription", color = TextMuted, fontSize = 12.sp, fontWeight = FontWeight.Bold)
                    Row(verticalAlignment = Alignment.Bottom) {
                        Text(daysLeft, color = TextWhite, fontSize = 48.sp, fontWeight = FontWeight.Bold)
                        Text(" days left", color = TextMuted, fontSize = 16.sp, modifier = Modifier.padding(bottom = 8.dp))
                    }
                }
                Icon(Icons.Default.Refresh, contentDescription = "Revoke", tint = Color.Red)
            }

            Spacer(modifier = Modifier.height(16.dp))

            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PlanButton(title = "Go", days = "7d", price = "₹10", modifier = Modifier.weight(1f)) { onPlanClick("go") }
                PlanButton(title = "Plus", days = "21d", price = "₹25", modifier = Modifier.weight(1f)) { onPlanClick("plus") }
                PlanButton(title = "Pro", days = "41d", price = "₹45", modifier = Modifier.weight(1f)) { onPlanClick("pro") }
            }
        }
    }
}

@Composable
fun PlanButton(title: String, days: String, price: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    Column(
        modifier = modifier.background(BgBlack, RoundedCornerShape(8.dp))
            .border(1.dp, BorderGray, RoundedCornerShape(8.dp))
            .clickable { onClick() }
            .padding(12.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text(title, color = TextWhite, fontWeight = FontWeight.Bold)
        Text(days, color = TextMuted, fontSize = 12.sp)
        Spacer(modifier = Modifier.height(8.dp))
        Text(price, color = AccentGreen, fontWeight = FontWeight.Bold)
    }
}

@Composable
fun QrCodeDialog(base64: String, amount: String, cookie: String, onDismiss: () -> Unit) {
    var status by remember { mutableStateOf("Awaiting payment...") }
    
    LaunchedEffect(Unit) {
        while(true) {
            val json = JSONObject().put("fingerprint", amount)
            val body = json.toString().toRequestBody("application/json".toMediaType())
            val req = Request.Builder().url("https://auto-payment.ximanta.xyz/api/recharge/verify").header("Cookie", cookie).post(body).build()
            
            var success = false
            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    val resBody = response.body?.string()
                    if (resBody != null && resBody.contains("exact")) {
                        success = true
                    }
                }
            }
            if (success) {
                status = "Payment Successful!"
                break
            }
            kotlinx.coroutines.delay(3000)
        }
    }

    Dialog(onDismissRequest = onDismiss) {
        Column(
            modifier = Modifier.background(SurfaceDark, RoundedCornerShape(16.dp)).border(1.dp, BorderGray, RoundedCornerShape(16.dp)).padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text("Recharge Plan", color = TextWhite, fontSize = 20.sp, fontWeight = FontWeight.Bold)
            Text("Pay exactly ₹$amount", color = AccentYellow, fontSize = 16.sp, modifier = Modifier.padding(vertical = 8.dp))
            
            val bitmap = BitmapFactory.decodeByteArray(Base64.decode(base64, Base64.DEFAULT), 0, Base64.decode(base64, Base64.DEFAULT).size)
            if (bitmap != null) {
                Image(bitmap = bitmap.asImageBitmap(), contentDescription = "QR", modifier = Modifier.size(200.dp).background(Color.White).padding(8.dp))
            }
            
            Text(status, color = AccentBlue, modifier = Modifier.padding(top = 16.dp))
            if (status == "Payment Successful!") {
                Button(onClick = onDismiss, modifier = Modifier.padding(top = 16.dp)) {
                    Text("Close", color = BgBlack)
                }
            } else {
                TextButton(onClick = onDismiss) {
                    Text("Cancel", color = AccentRed)
                }
            }
        }
    }
}

data class HistoryItem(val event: String, val amount: String, val status: String, val timestamp: Long)

@Composable
fun HistoryScreen(cookie: String) {
    var historyList by remember { mutableStateOf<List<HistoryItem>>(emptyList()) }
    var isLoading by remember { mutableStateOf(true) }

    LaunchedEffect(cookie) {
        val request = Request.Builder().url("https://auto-payment.ximanta.xyz/api/history").header("Cookie", cookie).build()
        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) { isLoading = false }
            override fun onResponse(call: Call, response: Response) {
                isLoading = false
                response.body?.string()?.let { body ->
                    try {
                        val jsonArray = JSONArray(body)
                        val items = mutableListOf<HistoryItem>()
                        for (i in 0 until jsonArray.length()) {
                            val obj = jsonArray.getJSONObject(i)
                            items.add(HistoryItem(obj.optString("event", "Unknown"), obj.optString("amount", "0.00"), obj.optString("status", "unknown"), obj.optLong("timestamp", 0)))
                        }
                        historyList = items
                    } catch (e: Exception) {}
                }
            }
        })
    }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        if (isLoading) {
            Text("Loading history...", color = TextMuted)
        } else if (historyList.isEmpty()) {
            Text("No recent activity.", color = TextMuted)
        } else {
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(historyList) { item -> HistoryRow(item) }
            }
        }
    }
}

@Composable
fun HistoryRow(item: HistoryItem) {
    val statusColor = when (item.status) {
        "exact" -> AccentGreen
        "partial", "overpaid" -> AccentYellow
        "not_found", "error" -> AccentRed
        else -> TextMuted
    }
    val timeString = try { SimpleDateFormat("dd MMM, HH:mm", Locale.getDefault()).format(Date(item.timestamp * 1000)) } catch (e: Exception) { "Unknown" }

    Row(
        modifier = Modifier.fillMaxWidth().background(SurfaceDark, RoundedCornerShape(8.dp))
            .border(1.dp, BorderGray, RoundedCornerShape(8.dp)).padding(16.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column {
            Text(item.event, color = TextWhite, fontWeight = FontWeight.Medium)
            Text(timeString, color = TextMuted, fontSize = 12.sp)
        }
        Column(horizontalAlignment = Alignment.End) {
            Text("₹${item.amount}", color = TextWhite, fontWeight = FontWeight.Bold)
            Text(item.status, color = statusColor, fontSize = 12.sp, fontWeight = FontWeight.Bold)
        }
    }
}

@Composable
fun ProfileScreen(email: String, onLogout: () -> Unit) {
    val name = email.substringBefore("@").replace(".", " ").split(" ").joinToString(" ") { 
        it.replaceFirstChar { char -> char.uppercase() } 
    }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Spacer(modifier = Modifier.height(24.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                modifier = Modifier.size(64.dp).background(AccentBlue, RoundedCornerShape(50)),
                contentAlignment = Alignment.Center
            ) {
                Text(name.take(1), color = Color.White, fontSize = 32.sp, fontWeight = FontWeight.Bold)
            }
            Spacer(modifier = Modifier.width(16.dp))
            Column {
                Text(name, color = TextWhite, fontSize = 24.sp, fontWeight = FontWeight.Bold)
                Text(email, color = TextMuted, fontSize = 14.sp)
            }
        }
        Spacer(modifier = Modifier.height(48.dp))
        Button(
            onClick = onLogout,
            colors = ButtonDefaults.buttonColors(containerColor = AccentRed),
            modifier = Modifier.fillMaxWidth().height(50.dp),
            shape = RoundedCornerShape(12.dp)
        ) {
            Text("Logout", color = Color.White, fontSize = 16.sp, fontWeight = FontWeight.Bold)
        }
    }
}

@Composable
fun BottomNavBar(currentScreen: String, onScreenChange: (String) -> Unit) {
    NavigationBar(containerColor = SurfaceDark) {
        NavigationBarItem(
            icon = { Icon(Icons.Default.Home, contentDescription = "Home") },
            label = { Text("Home") },
            selected = currentScreen == "home",
            onClick = { onScreenChange("home") },
            colors = NavigationBarItemDefaults.colors(selectedIconColor = TextWhite, selectedTextColor = TextWhite, unselectedIconColor = TextMuted, unselectedTextColor = TextMuted, indicatorColor = Color.Transparent)
        )
        NavigationBarItem(
            icon = { Icon(Icons.Default.List, contentDescription = "History") },
            label = { Text("History") },
            selected = currentScreen == "history",
            onClick = { onScreenChange("history") },
            colors = NavigationBarItemDefaults.colors(selectedIconColor = TextWhite, selectedTextColor = TextWhite, unselectedIconColor = TextMuted, unselectedTextColor = TextMuted, indicatorColor = Color.Transparent)
        )
        NavigationBarItem(
            icon = { Icon(Icons.Default.Person, contentDescription = "Profile") },
            label = { Text("Profile") },
            selected = currentScreen == "profile",
            onClick = { onScreenChange("profile") },
            colors = NavigationBarItemDefaults.colors(selectedIconColor = TextWhite, selectedTextColor = TextWhite, unselectedIconColor = TextMuted, unselectedTextColor = TextMuted, indicatorColor = Color.Transparent)
        )
    }
}
