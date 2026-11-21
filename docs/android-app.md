# UNIFUSION Android app (phase 1)

Sideload APK that runs `fieldd` on the phone without Termux and shows `/living`
in a WebView. This is phase 1 of `unifusion-app-plan.md`: **no Gemma**, no
packaged model, no on-device insight worker. Submit a file and approve or hold
it offline.

It is **not** a verified Play Store product, and it does not replace the
existing Termux install. A Termux-free inference engine is still unverified.

## What it does

- Starts a foreground service that execs the bundled arm64 `libfieldd.so`
  (the `fieldd` binary) on `127.0.0.1:8899`.
- Opens `/living` in a WebView (carbon black `#0a0a0a`, royal blue `#2f6bff`).
- Uses an **app-private** ledger under the app's files directory
  (`…/files/field`). It never reads or writes the Termux `.field` tree.
- Injects the fieldd bearer token the same way loopback `/living` already does.
  The app does not display or log the token value.
- Submit (`POST /sources`) records a `SOURCE_UPLOAD` and stops at AtGOV.
  Accept still requires a reason and a confirm press, then HUMAN_FINAL + REG.

## Install (sideload)

1. Copy `unifusion-android-v1-phase1-arm64-debug.apk` to the phone, for example
   into **Download**. (If you got it from a GitHub Release or Actions artifact
   on this branch, save that file. The APK is not stored in the git tree.)
2. Open the APK in Files / Downloads. Allow install from that source if Android
   asks.
3. Open **UNIFUSION**. Allow notifications if asked, and allow the app to ignore
   battery optimizations so the runtime can keep running.
4. Wait until `/living` appears. Submit a small text file, then Accept or Hold
   it from **Next Governed Step**. The first Submit press arms the button; press
   again to confirm.

The package is `com.unifusion.app`. Uninstalling the app deletes its private
ledger. It does not uninstall or modify Termux.

## Build

From the repository root, with a JDK 17+ and an Android SDK (platform 34):

```sh
export JAVA_HOME=/path/to/jdk17
export ANDROID_SDK_ROOT=/path/to/android-sdk
./scripts/build-android-app.sh
```

The script cross-compiles `cmd/fieldd` as `android/arm64` into
`android/app/src/main/jniLibs/arm64-v8a/libfieldd.so` (executable, packaged as
a native library so Android will extract it to an executable path), then runs
`gradlew assembleDebug`. The APK is written to
`android/app/build/outputs/apk/debug/app-debug.apk`.

`libfieldd.so` and the APK are build outputs; they are not committed.

## Safety

- No Termux, no shared-storage ledger, no auto HUMAN_FINAL.
- Phase 1 does not start `llama-server` or load a GGUF.
- Secrets, personal ledgers, and the phone's existing `.field` evidence stay
  out of the repository and out of the APK.
