#!/bin/bash
# Cross-compile fieldd for Android arm64 and build the sideload APK.
# Requires JDK 17+ (JAVA_HOME) and an Android SDK (ANDROID_SDK_ROOT or ANDROID_HOME).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 1
fi

sdk="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}"
if [ -z "$sdk" ] || [ ! -d "$sdk" ]; then
  echo "set ANDROID_SDK_ROOT to an Android SDK (platform 34, build-tools 34)" >&2
  exit 1
fi
export ANDROID_SDK_ROOT="$sdk"
export ANDROID_HOME="$sdk"

if [ -z "${JAVA_HOME:-}" ]; then
  if command -v /usr/libexec/java_home >/dev/null 2>&1; then
    JAVA_HOME="$(/usr/libexec/java_home -v 17 2>/dev/null || true)"
  fi
fi
if [ -n "${JAVA_HOME:-}" ]; then
  export JAVA_HOME
  export PATH="$JAVA_HOME/bin:$PATH"
fi

jni="$root/android/app/src/main/jniLibs/arm64-v8a"
mkdir -p "$jni"
echo "building libfieldd.so (GOOS=android GOARCH=arm64)"
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$jni/libfieldd.so" ./cmd/fieldd
chmod +x "$jni/libfieldd.so"

printf 'sdk.dir=%s\n' "$sdk" > "$root/android/local.properties"

cd "$root/android"
jar="$root/android/gradle/wrapper/gradle-wrapper.jar"
if [ ! -f "$jar" ]; then
  echo "downloading gradle-wrapper.jar"
  mkdir -p "$(dirname "$jar")"
  curl -fsSL -o "$jar" https://raw.githubusercontent.com/gradle/gradle/v8.7.0/gradle/wrapper/gradle-wrapper.jar
fi
if [ ! -x ./gradlew ]; then
  echo "android/gradlew is missing" >&2
  exit 1
fi
./gradlew --no-daemon assembleDebug assembleRelease
echo "debug APK: $root/android/app/build/outputs/apk/debug/app-debug.apk"
echo "release APK (debug-signed): $root/android/app/build/outputs/apk/release/app-release.apk"
