package com.ximanta.autopay

import android.annotation.SuppressLint
import android.content.Context
import android.os.Bundle
import android.webkit.CookieManager
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import okhttp3.*
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.util.concurrent.TimeUnit

// --- THEME COLORS ---
val BgBlack = Color(0xFF000000)
val SurfaceDark = Color(0xFF0a0a0a)
val BorderGray = Color(0xFF222222)
val TextWhite = Color(0xFFededed)
val TextMuted = Color(0xFF888888)
val AccentYellow = Color(0xFFfacc15)
val AccentGreen = Color(0xFF22c55e)

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

@Composable
fun AutoPayApp() {
    val context = LocalContext.current
    val prefs = context.getSharedPreferences("autopay_prefs", Context.MODE_PRIVATE)
    
    var sessionCookie by remember { mutableStateOf(prefs.getString("session_cookie", null)) }
    var showWebView by remember { mutableStateOf(false) }
    var currentScreen by remember { mutableStateOf("home") }

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
        Scaffold(
            containerColor = BgBlack,
            bottomBar = {
                if (sessionCookie != null) {
                    BottomNavBar(currentScreen) { currentScreen = it }
                }
            }
        ) { padding ->
            Box(modifier = Modifier.padding(padding).fillMaxSize()) {
                if (sessionCookie == null) {
                    LandingScreen(onSignInClick = { showWebView = true })
                } else {
                    when (currentScreen) {
                        "home" -> DashboardScreen(sessionCookie!!)
                        "history" -> HistoryScreen(sessionCookie!!)
                        "profile" -> ProfileScreen(sessionCookie!!, onLogout = {
                            prefs.edit().remove("session_cookie").apply()
                            sessionCookie = null
                        })
                    }
                }
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
                    CookieManager.getInstance().setAcceptCookie(true)
                    
                    webViewClient = object : WebViewClient() {
                        override fun onPageFinished(view: WebView?, url: String?) {
                            super.onPageFinished(view, url)
                            // Detect when the user is redirected to the dashboard
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
fun DashboardScreen(cookie: String) {
    var apiKey by remember { mutableStateOf("Loading...") }
    var daysLeft by remember { mutableStateOf("0") }
    val context = LocalContext.current

    // Fetch user data on launch
    LaunchedEffect(cookie) {
        val request = Request.Builder()
            .url("https://auto-payment.ximanta.xyz/api/me")
            .header("Cookie", cookie)
            .build()

        client.newCall(request).enqueue(object : Callback {
            override fun onFailure(call: Call, e: IOException) {
                apiKey = "Network Error"
            }

            override fun onResponse(call: Call, response: Response) {
                response.body?.string()?.let { body ->
                    try {
                        val json = JSONObject(body)
                        apiKey = json.optString("api_key", "No Key")
                        daysLeft = json.optString("days_left", "0")
                    } catch (e: Exception) {
                        apiKey = "Parse Error"
                    }
                }
            }
        })
    }

    Column(
        modifier = Modifier.fillMaxSize().padding(16.dp)
    ) {
        // Top Bar
        Row(
            modifier = Modifier.fillMaxWidth().padding(bottom = 24.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(Icons.Default.Menu, contentDescription = "Menu", tint = TextWhite)
            Icon(Icons.Default.Notifications, contentDescription = "Notifications", tint = TextWhite)
        }

        // API Key Card
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(SurfaceDark, RoundedCornerShape(12.dp))
                .border(1.dp, BorderGray, RoundedCornerShape(12.dp))
                .padding(16.dp)
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
                    val clip = android.content.ClipData.newPlainText("API Key", apiKey)
                    clipboard.setPrimaryClip(clip)
                }) {
                    Icon(Icons.Default.ContentCopy, contentDescription = "Copy", tint = TextMuted)
                }
            }
        }

        Spacer(modifier = Modifier.height(16.dp))

        // Subscription Card
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(SurfaceDark, RoundedCornerShape(12.dp))
                .border(1.dp, BorderGray, RoundedCornerShape(12.dp))
                .padding(16.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
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

            // Plans
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PlanButton(title = "Go", days = "7d", price = "₹10", modifier = Modifier.weight(1f))
                PlanButton(title = "Plus", days = "21d", price = "₹25", modifier = Modifier.weight(1f))
                PlanButton(title = "Pro", days = "41d", price = "₹45", modifier = Modifier.weight(1f))
            }
        }
    }
}

@Composable
fun PlanButton(title: String, days: String, price: String, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .background(BgBlack, RoundedCornerShape(8.dp))
            .border(1.dp, BorderGray, RoundedCornerShape(8.dp))
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
fun HistoryScreen(cookie: String) {
    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("History", color = TextWhite, fontSize = 24.sp, fontWeight = FontWeight.Bold)
        Spacer(modifier = Modifier.height(16.dp))
        Text("Loading history...", color = TextMuted)
    }
}

@Composable
fun ProfileScreen(cookie: String, onLogout: () -> Unit) {
    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("Profile", color = TextWhite, fontSize = 24.sp, fontWeight = FontWeight.Bold)
        Spacer(modifier = Modifier.height(16.dp))
        Button(onClick = onLogout, colors = ButtonDefaults.buttonColors(containerColor = Color.Red)) {
            Text("Logout", color = TextWhite)
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
