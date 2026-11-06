package com.papacs.snaptrans.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.papacs.snaptrans.config.AppSettings

private val Blue = Color(0xFF2563EB)
private val Ink = Color(0xFF0F172A)
private val Muted = Color(0xFF64748B)
private val Canvas = Color(0xFFF7F8FC)
private val Success = Color(0xFF059669)

private val colors = lightColorScheme(
    primary = Blue,
    onPrimary = Color.White,
    background = Canvas,
    onBackground = Ink,
    surface = Color.White,
    onSurface = Ink,
    outline = Color(0xFFCBD5E1),
)

@Composable
fun SnapTransApp(
    settings: AppSettings,
    overlayGranted: Boolean,
    serviceRunning: Boolean,
    message: String?,
    onMessageShown: () -> Unit,
    onSettingsChanged: (AppSettings) -> Unit,
    onGrantOverlay: () -> Unit,
    onSave: () -> Unit,
    onStart: () -> Unit,
    onStop: () -> Unit,
) {
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(message) {
        if (!message.isNullOrBlank()) {
            snackbar.showSnackbar(message)
            onMessageShown()
        }
    }
    MaterialTheme(colorScheme = colors) {
        Scaffold(
            containerColor = Canvas,
            snackbarHost = { SnackbarHost(snackbar) },
        ) { contentPadding ->
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(contentPadding)
                    .statusBarsPadding()
                    .navigationBarsPadding()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 20.dp, vertical = 18.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Hero(serviceRunning)
                PermissionCard(overlayGranted, onGrantOverlay)
                ConnectionCard(settings, onSettingsChanged, onSave)
                EfficiencyCard()
                Button(
                    onClick = onStart,
                    enabled = !serviceRunning,
                    modifier = Modifier.fillMaxWidth().height(54.dp),
                    shape = RoundedCornerShape(16.dp),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = Blue,
                        disabledContainerColor = Color(0xFFD5DDEC),
                    ),
                ) {
                    Text(
                        if (serviceRunning) "屏幕翻译正在运行" else "启动屏幕翻译",
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                }
                if (serviceRunning) {
                    OutlinedButton(
                        onClick = onStop,
                        modifier = Modifier.fillMaxWidth().height(50.dp),
                        shape = RoundedCornerShape(16.dp),
                    ) { Text("停止服务并释放截屏授权") }
                }
                Text(
                    "启动后可离开本页。点击屏幕边缘的“译”开始，点击“×”恢复原页面。",
                    color = Muted,
                    fontSize = 13.sp,
                    lineHeight = 19.sp,
                    modifier = Modifier.padding(horizontal = 4.dp, vertical = 2.dp),
                )
            }
        }
    }
}

@Composable
private fun Hero(running: Boolean) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Box(
            modifier = Modifier.size(54.dp).background(Blue, RoundedCornerShape(17.dp)),
            contentAlignment = Alignment.Center,
        ) {
            Text("译", color = Color.White, fontSize = 25.sp, fontWeight = FontWeight.Bold)
        }
        Column(modifier = Modifier.weight(1f)) {
            Text("snapTrans", color = Ink, fontSize = 25.sp, fontWeight = FontWeight.Bold)
            Text("一触即译，停留在正在阅读的地方", color = Muted, fontSize = 13.sp)
        }
        StatusPill(running)
    }
}

@Composable
private fun StatusPill(running: Boolean) {
    val color = if (running) Success else Muted
    Row(
        modifier = Modifier.background(color.copy(alpha = 0.10f), CircleShape)
            .padding(horizontal = 10.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Box(Modifier.size(7.dp).background(color, CircleShape))
        Text(if (running) "运行中" else "未启动", color = color, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
private fun PermissionCard(granted: Boolean, onGrant: () -> Unit) {
    AppCard {
        Text("使用准备", fontWeight = FontWeight.Bold, fontSize = 17.sp)
        Spacer(Modifier.height(12.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(30.dp).background(
                    if (granted) Success.copy(alpha = 0.12f) else Blue.copy(alpha = 0.10f),
                    CircleShape,
                ),
                contentAlignment = Alignment.Center,
            ) { Text(if (granted) "✓" else "1", color = if (granted) Success else Blue, fontWeight = FontWeight.Bold) }
            Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                Text("显示在其他应用上层", fontWeight = FontWeight.Medium)
                Text(if (granted) "已授权" else "用于悬浮球和原位译文", color = Muted, fontSize = 12.sp)
            }
            if (!granted) TextButton(onClick = onGrant) { Text("去授权") }
        }
        Spacer(Modifier.height(10.dp))
        Text(
            "启动时 Android 会再询问一次截屏授权。画面只在本机内存中识别，不会上传截图。",
            color = Muted,
            fontSize = 12.sp,
            lineHeight = 18.sp,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ConnectionCard(
    settings: AppSettings,
    onChange: (AppSettings) -> Unit,
    onSave: () -> Unit,
) {
    var showKey by remember { mutableStateOf(false) }
    AppCard {
        Text("翻译服务", fontWeight = FontWeight.Bold, fontSize = 17.sp)
        Text("固定英文 → 简体中文", color = Blue, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(12.dp))
        OutlinedTextField(
            value = settings.apiKey,
            onValueChange = { onChange(settings.copy(apiKey = it)) },
            modifier = Modifier.fillMaxWidth(),
            label = { Text("API Key") },
            singleLine = true,
            visualTransformation = if (showKey) VisualTransformation.None else PasswordVisualTransformation(),
            trailingIcon = {
                TextButton(onClick = { showKey = !showKey }) { Text(if (showKey) "隐藏" else "显示") }
            },
            shape = RoundedCornerShape(13.dp),
        )
        Spacer(Modifier.height(10.dp))
        OutlinedTextField(
            value = settings.baseUrl,
            onValueChange = { onChange(settings.copy(baseUrl = it)) },
            modifier = Modifier.fillMaxWidth(),
            label = { Text("API Base URL") },
            singleLine = true,
            shape = RoundedCornerShape(13.dp),
        )
        Spacer(Modifier.height(10.dp))
        OutlinedTextField(
            value = settings.model,
            onValueChange = { onChange(settings.copy(model = it)) },
            modifier = Modifier.fillMaxWidth(),
            label = { Text("模型") },
            singleLine = true,
            shape = RoundedCornerShape(13.dp),
        )
        Spacer(Modifier.height(8.dp))
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TextButton(onClick = onSave) { Text("保存设置") }
        }
        Text("API Key 使用 Android Keystore 加密；发送内容仅包含筛选后的英文文字。", color = Muted, fontSize = 12.sp)
    }
}

@Composable
private fun EfficiencyCard() {
    AppCard {
        Text("已启用的效率策略", fontWeight = FontWeight.Bold, fontSize = 17.sp)
        Spacer(Modifier.height(10.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            MiniChip("本地 OCR")
            MiniChip("短提示词")
            MiniChip("重复缓存")
        }
        Spacer(Modifier.height(10.dp))
        Text(
            "每屏最多处理 80 个英文块，自动过滤网址、数字和非英文内容；常见按钮词直接在本地翻译。",
            color = Muted,
            fontSize = 12.sp,
            lineHeight = 18.sp,
        )
    }
}

@Composable
private fun MiniChip(text: String) {
    Surface(color = Color(0xFFEFF6FF), shape = CircleShape) {
        Text(text, color = Blue, fontSize = 11.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(horizontal = 9.dp, vertical = 6.dp))
    }
}

@Composable
private fun AppCard(content: @Composable ColumnScope.() -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = Color.White),
        elevation = CardDefaults.cardElevation(defaultElevation = 1.dp),
    ) {
        Column(Modifier.fillMaxWidth().padding(18.dp), content = content)
    }
}
