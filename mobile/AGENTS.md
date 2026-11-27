# swcode's Android app: notes

The web app in a Capacitor WebView (`README.md` here says what it is and how it is built). The pages are the root's
`web/`; what they do differently in the app is behind `inAndroidApp()` (`web/src/services/native-app.ts`).

## Run and check

- Nothing here compiles on the development machine: its JDK has no `javac` and there is no Android SDK. A Java,
  manifest or Gradle change is checked by `gh workflow run android.yml --ref main` (about 2 minutes), which keeps
  the APK as the run's `swcode-android` artifact and skips the release upload because the ref is not a tag.
- A manifest change is only in an APK that workflow built: read it back with
  `unzip -p swcode-android.apk AndroidManifest.xml | strings -a -el | grep permission`.
- `npm run sync` (here) copies the root's `dist/web` into `www`, so `npm run build` at the root comes first.
- `.github/workflows/android.yml` builds the APK from the tag's release, apart from `release.yml`, and signs it
  with the four `ANDROID_*` secrets: without them the app can be neither installed nor updated.

## Where things are

- `android/app/src/main/java/com/bioleyl/swcode/`: `MainActivity` registers the app's own plugins before
  `super.onCreate`; each has its page-side gateway in `web/src/services/`:
  - `UpdaterPlugin` + `Updater` (`app-update.ts`): the look for a release, the download, the checksum, the installer.
  - `NotifierPlugin` + `NotifierService` (`native-notifications.ts`): Android's notices and the foreground service.
  - `MicrophonePlugin` (`native-microphone.ts`): Android's question about the microphone, for dictation.
- `android/app/build.gradle` takes the version from `-PswcodeVersion` / `-PswcodeVersionCode`.
- `scripts/sync.mjs` leaves out the `.gz` and `.br` copies of `dist/web`: Android's asset merger takes each pair as
  one resource and fails the APK with "Duplicate resources".

## Rules

- `server.androidScheme` stays `'http'` (with `cleartext`): from `https://localhost` the WebView blocks a
  plain-HTTP daemon as mixed content. `http://localhost` is still a secure context.
- The notices are native, not Web Push (a WebView has no push service): the page feeds the daemon's socket through
  `web/src/lib/notice-tracker.ts`, a mirror of `desktop/watch.Tracker`, to `Notifier.show`. A tapped notice's
  session rides on the launch intent's `swcode.session` extra, which the page takes with `Notifier.takeSession()`.
- `NotifierService` keeps the process off Android's cached-app freezer so the WebView's socket keeps speaking while
  the phone sleeps. It dies with the process: a kill by Android ends the notices.
- The updater looks on `onResume` (the network at most every `LOOK_EVERY_MS`), since the app usually comes back
  rather than opening anew. A look the person asks for skips the daily `mayAsk` gate.
- The first launch shows `sc-connect`: `/api/status` on Capacitor's local server answers the app's HTML, which
  reads as `badAnswer`.

## Traps

- **Whatever the page uses of the device needs its permission in the manifest, and someone to ask for it.** The
  camera (`getUserMedia`) goes through Capacitor's `onPermissionRequest`, which asks once `CAMERA` is declared and
  is denied at once when it is not. Speech recognition does not go through it at all: Android's recognizer refuses
  an app without `RECORD_AUDIO` (`not-allowed`) and nothing asks, so the page asks first through
  `Microphone.requestPermissions()`. The page words such a refusal for the app (Android's settings, not a site's).
- The WebView finds the speech recognizer by the `android.speech.RecognitionService` action: Android 11+ hides
  other apps' services without the manifest's `<queries>` entry for it.
- A plugin with no method of its own still answers `checkPermissions` / `requestPermissions` to the page, by the
  aliases of its `@CapacitorPlugin(permissions = …)`. Capacitor 8's `@Permission` takes `strings = { … }`. A plugin
  method that must ask calls `requestAllPermissions(call, "callbackName")` and resolves the call in a
  `@PermissionCallback` of that name; a denial still runs the callback.
- Android 14+ throws `SecurityException` from `startForeground(…, CONNECTED_DEVICE)` unless the manifest declares
  `FOREGROUND_SERVICE_CONNECTED_DEVICE` and one of the type's prerequisites (`CHANGE_NETWORK_STATE` here). The
  service starts at launch, so a missing one takes the app down as it opens.
- In Gradle's Groovy, `versionCode (expr).toInteger()` passes the String to `versionCode` (the call binds before
  `.toInteger()`), and AGP answers `Value is null`: assign to a `def` first.
- Capacitor overwrites `window.Capacitor` when its core loads, so an e2e init script cannot set `isNativePlatform`:
  it sets `window.androidBridge = {}`, which Capacitor reads as Android.
