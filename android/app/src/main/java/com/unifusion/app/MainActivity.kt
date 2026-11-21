package com.unifusion.app

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.view.View
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors

class MainActivity : AppCompatActivity() {

    private var filePathCallback: ValueCallback<Array<Uri>>? = null
    private lateinit var webView: WebView
    private lateinit var starting: LinearLayout
    private lateinit var startingText: TextView
    private val io = Executors.newSingleThreadExecutor()
    private var livingLoaded = false
    private var shownError: String = ""

    private val fileChooser = registerForActivityResult(
        ActivityResultContracts.GetMultipleContents(),
    ) { uris ->
        val cb = filePathCallback
        filePathCallback = null
        cb?.onReceiveValue(uris?.toTypedArray() ?: emptyArray())
    }

    private val notifyPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { startRuntime() }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        webView = findViewById(R.id.living)
        starting = findViewById(R.id.starting)
        startingText = findViewById(R.id.startingText)
        setupWebView()
        maybeAskPermissions()
    }

    override fun onDestroy() {
        // Leave the foreground service running so fieldd stays up.
        io.shutdownNow()
        super.onDestroy()
    }

    @Deprecated("Use OnBackPressedDispatcher")
    override fun onBackPressed() {
        if (this::webView.isInitialized && webView.canGoBack()) {
            webView.goBack()
        } else {
            super.onBackPressed()
        }
    }

    private fun maybeAskPermissions() {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            android.content.pm.PackageManager.PERMISSION_GRANTED
        ) {
            notifyPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        } else {
            startRuntime()
        }
    }

    private fun startRuntime() {
        requestBatteryExemption()
        ContextCompat.startForegroundService(
            this,
            Intent(this, FielddService::class.java),
        )
        waitForFieldd()
    }

    private fun requestBatteryExemption() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return
        val pm = getSystemService(PowerManager::class.java) ?: return
        if (pm.isIgnoringBatteryOptimizations(packageName)) return
        try {
            val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
            intent.data = Uri.parse("package:$packageName")
            startActivity(intent)
        } catch (_: Exception) {
            // The settings screen is optional; the foreground service still runs.
        }
    }

    private fun waitForFieldd() {
        io.execute {
            var attempt = 0
            while (!isDestroyed && !isFinishing) {
                attempt++
                val port = FielddService.listenPort.get()
                if (port > 0 && healthOk(port)) {
                    runOnUiThread { showLiving(port) }
                    return@execute
                }
                val err = FielddService.lastError.get()
                if (err.isNotEmpty() && err != shownError) {
                    shownError = err
                    runOnUiThread { showStartupMessage(err) }
                } else if (attempt == 10 && shownError.isEmpty()) {
                    runOnUiThread {
                        showStartupMessage(getString(R.string.still_starting))
                    }
                } else if (attempt == 25 && shownError.isEmpty()) {
                    runOnUiThread {
                        showStartupMessage(getString(R.string.start_timeout))
                    }
                }
                try {
                    Thread.sleep(400)
                } catch (_: InterruptedException) {
                    return@execute
                }
            }
        }
    }

    private fun healthOk(port: Int): Boolean {
        return try {
            val conn = URL(FielddService.healthUrl(port)).openConnection() as HttpURLConnection
            conn.connectTimeout = 800
            conn.readTimeout = 800
            conn.useCaches = false
            conn.requestMethod = "GET"
            val code = conn.responseCode
            conn.disconnect()
            code == 200
        } catch (_: Exception) {
            false
        }
    }

    private fun showStartupMessage(msg: String) {
        if (isDestroyed) return
        livingLoaded = false
        webView.visibility = View.INVISIBLE
        starting.visibility = View.VISIBLE
        startingText.text = msg
    }

    private fun showLiving(port: Int) {
        if (isDestroyed || livingLoaded) return
        livingLoaded = true
        shownError = ""
        starting.visibility = View.GONE
        webView.visibility = View.VISIBLE
        webView.loadUrl(FielddService.livingUrl(port))
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun setupWebView() {
        val s = webView.settings
        s.javaScriptEnabled = true
        s.domStorageEnabled = true
        s.allowFileAccess = false
        s.allowContentAccess = true
        s.cacheMode = WebSettings.LOAD_NO_CACHE
        s.mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
        webView.setBackgroundColor(0xFF0A0A0A.toInt())
        webView.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                val host = request.url.host
                val scheme = request.url.scheme
                val port = request.url.port
                val expected = FielddService.listenPort.get()
                // Stay on the local fieldd surface. Never send the loopback
                // session to a remote site.
                val local = scheme == "http" && (host == "127.0.0.1" || host == "localhost")
                val samePort = expected <= 0 || port == -1 || port == expected
                return !(local && samePort)
            }

            override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
                if (request.isForMainFrame) {
                    val err = FielddService.lastError.get().ifEmpty {
                        getString(R.string.fieldd_restarting)
                    }
                    showStartupMessage(err)
                    waitForFieldd()
                }
            }
        }
        webView.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(
                view: WebView?,
                callback: ValueCallback<Array<Uri>>?,
                params: FileChooserParams?,
            ): Boolean {
                filePathCallback?.onReceiveValue(null)
                filePathCallback = callback
                return try {
                    fileChooser.launch("*/*")
                    true
                } catch (_: Exception) {
                    filePathCallback = null
                    callback?.onReceiveValue(null)
                    false
                }
            }
        }
    }
}
