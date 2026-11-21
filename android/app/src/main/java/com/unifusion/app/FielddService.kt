package com.unifusion.app

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.os.IBinder
import androidx.core.app.NotificationCompat
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference

/**
 * Runs the bundled fieldd binary (libfieldd.so) as a child process on a
 * loopback port that is not Termux fieldd (8899) or llama-server (8897).
 * The default is 8896; if that is taken, the next free candidate is used
 * and the WebView is pointed at that port. State lives under the app's
 * files directory only and never under Termux or shared storage.
 */
class FielddService : Service() {

    @Volatile private var child: java.lang.Process? = null
    private val stopping = AtomicBoolean(false)
    private var worker: Thread? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startForeground(NOTIFY_ID, notification())
        if (worker == null || worker?.isAlive != true) {
            stopping.set(false)
            worker = Thread({ runLoop() }, "fieldd-watch").also { it.start() }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        stopping.set(true)
        stopChild()
        super.onDestroy()
    }

    private fun notification(): Notification {
        val launch = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val port = listenPort.get()
        val text = if (port > 0) {
            getString(R.string.notification_text_port, port)
        } else {
            getString(R.string.notification_text)
        }
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.notification_title))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_stat_unifusion)
            .setContentIntent(launch)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setColor(0xFF2F6BFF.toInt())
            .build()
    }

    private fun runLoop() {
        var backoffMs = 1_000L
        while (!stopping.get()) {
            val code = try {
                startChild()
                child?.waitFor() ?: 0
            } catch (_: InterruptedException) {
                break
            } catch (e: Exception) {
                val msg = startFailureMessage(e)
                setLastError(msg)
                appendLog("fieldd start failed: $msg")
                -1
            }
            if (stopping.get()) break
            if (lastError.get().isEmpty()) {
                setLastError(
                    "The local runtime stopped (exit $code). " +
                        "The app will retry. It does not use Termux port 8899.",
                )
            }
            appendLog("fieldd exited status=$code; restarting")
            listenPort.set(0)
            try {
                Thread.sleep(backoffMs)
            } catch (_: InterruptedException) {
                break
            }
            backoffMs = (backoffMs * 2).coerceAtMost(15_000L)
        }
    }

    private fun startChild() {
        stopChild()
        val bin = File(applicationInfo.nativeLibraryDir, "libfieldd.so")
        if (!bin.exists()) {
            throw IllegalStateException("bundled fieldd is missing from the app package")
        }
        // nativeLibraryDir is executable; do not copy the binary into writable storage.
        val files = filesDir
        val cache = cacheDir
        val state = File(files, "field")
        val work = File(files, "field-work")
        val outputs = File(files, "outputs")
        val sources = File(files, "sources")
        listOf(state, work, outputs, sources, cache).forEach { it.mkdirs() }

        val port = pickLoopbackPort()
        val bind = "$HOST:$port"
        listenPort.set(port)
        setLastError("")
        writePortFile(cache, port)

        val pb = ProcessBuilder(
            bin.absolutePath,
            "-state", state.absolutePath,
            "-http", bind,
            "-work-dir", work.absolutePath,
            "-pid", File(cache, "fieldd.pid").absolutePath,
            "-socket", "off",
        )
        pb.directory(files)
        pb.redirectErrorStream(true)
        val env = pb.environment()
        env["HOME"] = files.absolutePath
        env["TMPDIR"] = cache.absolutePath
        env["UNIFUSION_SOURCES_DIR"] = sources.absolutePath
        // A fresh ledger. Never inherit a reasoner or token from the process environment.
        listOf(
            "FIELDD_TOKEN",
            "GEMMA_BIN",
            "GEMMA_MODEL",
            "GEMMA_MODEL_DIRS",
            "UNIFUSION_LLAMA_SERVER",
            "FIELD_HOME",
        ).forEach { env.remove(it) }

        val proc = pb.start()
        child = proc
        Thread({ copyLogs(proc, port) }, "fieldd-log").also { it.isDaemon = true; it.start() }
        startForeground(NOTIFY_ID, notification())
    }

    private fun copyLogs(proc: java.lang.Process, port: Int) {
        val log = File(cacheDir, "fieldd.log")
        try {
            proc.inputStream.bufferedReader().use { reader ->
                FileOutputStream(log, true).bufferedWriter().use { out ->
                    while (true) {
                        val line = reader.readLine() ?: break
                        val safe = redact(line)
                        out.appendLine(safe)
                        out.flush()
                        if (looksLikeBindFailure(safe)) {
                            setLastError(
                                "Could not listen on 127.0.0.1:$port. " +
                                    "Termux fieldd uses 8899 and llama-server uses 8897; " +
                                    "this app uses 8896 or the next free port. " +
                                    "Close the conflicting listener, then reopen the app.",
                            )
                        }
                    }
                }
            }
        } catch (_: Exception) {
            // Process ended or log unwritable; the supervisor will restart fieldd.
        }
    }

    private fun stopChild() {
        val proc = child ?: return
        child = null
        proc.destroy()
        val deadline = System.currentTimeMillis() + 3_000
        while (proc.isAlive && System.currentTimeMillis() < deadline) {
            try {
                Thread.sleep(50)
            } catch (_: InterruptedException) {
                break
            }
        }
        if (proc.isAlive) {
            proc.destroyForcibly()
        }
    }

    private fun appendLog(line: String) {
        try {
            File(cacheDir, "fieldd.log").appendText(redact(line) + "\n")
        } catch (_: Exception) {
        }
    }

    private fun setLastError(msg: String) {
        lastError.set(msg)
        try {
            File(cacheDir, "fieldd-error.txt").writeText(redact(msg))
        } catch (_: Exception) {
        }
    }

    private fun writePortFile(cache: File, port: Int) {
        try {
            File(cache, "fieldd.port").writeText(port.toString())
        } catch (_: Exception) {
        }
    }

    companion object {
        const val CHANNEL_ID = "unifusion-fieldd"
        const val HOST = "127.0.0.1"
        const val PREFERRED_PORT = 8896
        // Termux fieldd and the kept-loaded llama-server already used on the phone.
        val RESERVED_PORTS = setOf(8899, 8897)
        private const val NOTIFY_ID = 17

        val listenPort = AtomicInteger(0)
        val lastError = AtomicReference("")

        fun livingUrl(port: Int): String = "http://$HOST:$port/living"

        fun healthUrl(port: Int): String = "http://$HOST:$port/healthz"

        fun startFailureMessage(e: Exception): String {
            val detail = e.message?.trim().orEmpty()
            return if (detail.isNotEmpty()) {
                "The local runtime did not start: $detail"
            } else {
                "The local runtime did not start (${e.javaClass.simpleName})."
            }
        }

        fun noPortMessage(): String =
            "No free loopback port for the app runtime. " +
                "This app will not use 8899 (Termux fieldd) or 8897 (llama-server). " +
                "It tries 8896 first, then nearby ports."

        fun pickLoopbackPort(): Int {
            val preferred = listOf(8896, 8895, 8894, 8893, 8892, 8891, 8890, 8888, 8887)
            for (port in preferred) {
                if (port !in RESERVED_PORTS && portIsFree(port)) return port
            }
            for (port in 8900..8920) {
                if (port !in RESERVED_PORTS && portIsFree(port)) return port
            }
            throw IOException(noPortMessage())
        }

        fun portIsFree(port: Int): Boolean {
            return try {
                ServerSocket().use { socket ->
                    socket.reuseAddress = false
                    socket.bind(InetSocketAddress(HOST, port))
                }
                true
            } catch (_: IOException) {
                false
            }
        }

        fun looksLikeBindFailure(line: String): Boolean {
            val s = line.lowercase()
            return s.contains("http listen") && (
                s.contains("failed") ||
                    s.contains("address already in use") ||
                    s.contains("bind:")
                )
        }

        fun stateDir(files: File): File = File(files, "field")

        fun tokenFile(files: File): File = File(stateDir(files), "fieldd.token")

        // Drop obvious secret material if fieldd ever printed it. The token
        // value is never written here on purpose.
        fun redact(line: String): String {
            var s = line
            s = TOKEN_ASSIGN.replace(s, "$1=***")
            s = BEARER.replace(s, "$1 ***")
            return s
        }

        private val TOKEN_ASSIGN = Regex("(?i)(token|secret|password|authorization)\\s*[:=]\\s*\\S+")
        private val BEARER = Regex("(?i)(bearer)\\s+\\S+")
    }
}
