package com.unifusion.app

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.os.IBinder
import androidx.core.app.NotificationCompat
import java.io.File
import java.io.FileOutputStream
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Runs the bundled fieldd binary (libfieldd.so) as a child process on
 * 127.0.0.1:8899. State lives under the app's files directory only and
 * never under Termux or shared storage.
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
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.notification_title))
            .setContentText(getString(R.string.notification_text))
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
                appendLog("fieldd start failed: ${e.javaClass.simpleName}")
                -1
            }
            if (stopping.get()) break
            appendLog("fieldd exited status=$code; restarting")
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
            throw IllegalStateException("bundled fieldd is missing")
        }
        // nativeLibraryDir is executable; do not copy the binary into writable storage.
        val files = filesDir
        val cache = cacheDir
        val state = File(files, "field")
        val work = File(files, "field-work")
        val outputs = File(files, "outputs")
        val sources = File(files, "sources")
        listOf(state, work, outputs, sources, cache).forEach { it.mkdirs() }

        val pb = ProcessBuilder(
            bin.absolutePath,
            "-state", state.absolutePath,
            "-http", BIND,
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
        Thread({ copyLogs(proc) }, "fieldd-log").also { it.isDaemon = true; it.start() }
    }

    private fun copyLogs(proc: Process) {
        val log = File(cacheDir, "fieldd.log")
        try {
            proc.inputStream.bufferedReader().use { reader ->
                FileOutputStream(log, true).bufferedWriter().use { out ->
                    while (true) {
                        val line = reader.readLine() ?: break
                        out.appendLine(redact(line))
                        out.flush()
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

    companion object {
        const val CHANNEL_ID = "unifusion-fieldd"
        const val BIND = "127.0.0.1:8899"
        const val LIVING = "http://127.0.0.1:8899/living"
        const val HEALTH = "http://127.0.0.1:8899/healthz"
        private const val NOTIFY_ID = 17

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
