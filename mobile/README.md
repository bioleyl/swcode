# swcode's Android app

The web app in a WebView of its own, from the release's pages, so a phone has swcode in its launcher without a browser.
It pairs with a computer by number (`swcode pair <number>` there), keeps several computers, and takes its next release
from GitHub itself: `Updater.java` asks for the latest release, downloads `swcode-android.apk` when its version is
newer, checks the release's checksum, and hands the file to Android's installer.

Not on a store: the APK is installed from the release's page, and Android asks once to allow installing apps from
swcode.

## The pieces

- `www/` — the built pages, copied by `npm run sync` from the root's `dist/web`; not committed.
- `android/` — the Capacitor project, which `npm run sync` fills, and Gradle builds.
- `android/app/build.gradle` — the version comes from the build (`-PswcodeVersion`, `-PswcodeVersionCode`), and the
  release is signed with `android/keystore.properties` when it is there.
- `android/app/src/main/java/com/bioleyl/swcode/Updater.java` — the update check, the download and the installer.

## Building by hand

Needs JDK 21 and the Android SDK (compileSdk 36).

```bash
npm run build                 # at the repository's root: dist/web
cd mobile
npm ci
npm run sync                  # www → android, then Capacitor's sync
cd android
./gradlew assembleRelease -PswcodeVersion=0.0.0 -PswcodeVersionCode=1
```

Without `android/keystore.properties` the APK is unsigned, and Android refuses it. To sign by hand:

```bash
keytool -genkeypair -v -keystore swcode.keystore -alias swcode -keyalg RSA -keysize 2048 -validity 10000
printf 'storeFile=swcode.keystore\nstorePassword=%s\nkeyAlias=swcode\nkeyPassword=%s\n' "$PASSWORD" "$PASSWORD" \
  > android/keystore.properties
```

Keep that keystore: an app updated by a release signed with another key is refused, and the person has to uninstall it
first. `keystore.properties` and `*.keystore` are ignored by git.

## The workflow's secrets

`.github/workflows/android.yml` signs with four repository secrets:

- `ANDROID_KEYSTORE_BASE64` — `base64 -w0 swcode.keystore`
- `ANDROID_KEYSTORE_PASSWORD`
- `ANDROID_KEY_ALIAS`
- `ANDROID_KEY_PASSWORD`

It uploads `swcode-android.apk` and `swcode-android.apk.sha256` to the tag's release; the app takes them from there.
