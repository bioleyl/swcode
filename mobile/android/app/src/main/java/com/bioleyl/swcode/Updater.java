package com.bioleyl.swcode;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import android.util.Log;

import androidx.core.content.FileProvider;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.security.MessageDigest;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * The app updates itself, from the same GitHub release the daemon takes its updates from: it asks for the latest
 * release, downloads swcode-android.apk when its version is newer, checks the release's checksum, and hands the file
 * to Android's installer. Android asks once for permission to install apps from swcode, which opens here.
 *
 * It looks when the app opens or comes back, at most every ten minutes; Settings → Phone's "Look now" looks when the
 * person asks, and says what it found. A look that fails on its own reaches nobody: the app keeps running as it is.
 */
final class Updater {
    private static final String TAG = "swcode";
    private static final String RELEASES = "https://api.github.com/repos/bioleyl/swcode/releases/latest";
    private static final String APK = "swcode-android.apk";
    private static final String CHECKSUM = "swcode-android.apk.sha256";
    private static final String DIRECTORY = "updates";
    private static final String READY_VERSION = "readyVersion";
    private static final String ASKED_VERSION = "askedVersion";
    private static final String ASKED_AT = "askedAt";
    private static final String LOOKED_AT = "lookedAt";
    // The person who turned the installer down is asked again a day later, and not at every launch.
    private static final long ASK_AGAIN_MS = 24L * 60L * 60L * 1000L;
    // A look of its own, at each return to the app, asks GitHub at most this often.
    private static final long LOOK_EVERY_MS = 10L * 60L * 1000L;
    private static final int TIMEOUT_MS = 20_000;
    private static final int BUFFER = 8_192;
    private static final int PARTS = 3;
    private static final ExecutorService ONCE = Executors.newSingleThreadExecutor();

    private Updater() {}

    /** What the web app's look found: the version Android is installing, empty when the app is the latest. */
    interface Looked {
        void found(String version);

        void failed(String problem);
    }

    /** Looks for a newer release, and has Android install it: called as the app opens, and as it comes back. */
    static void check(Activity activity) {
        ONCE.execute(() -> {
            try {
                look(activity, false);
            } catch (Exception failure) {
                Log.w(TAG, "the update check found nothing it could use", failure);
            }
        });
    }

    /** Looks now, whatever the last look, and says what it found: the Settings → Phone's "Look now". */
    static void lookNow(Activity activity, Looked looked) {
        ONCE.execute(() -> {
            try {
                looked.found(look(activity, true));
            } catch (Exception failure) {
                Log.w(TAG, "the update check failed", failure);
                looked.failed(problemOf(failure));
            }
        });
    }

    /**
     * Looks for a newer release, downloads it, and has Android install it when the person may be asked. Gives the
     * version it asked to install, or empty when there is nothing newer to offer.
     */
    private static String look(Activity activity, boolean forced) throws Exception {
        SharedPreferences kept = activity.getPreferences(Activity.MODE_PRIVATE);
        String current = versionOf(activity);
        File file = new File(new File(activity.getFilesDir(), DIRECTORY), APK);
        String ready = kept.getString(READY_VERSION, "");
        if (!forced && !worthLooking(kept, ready, current, file)) {
            return "";
        }
        kept.edit().putLong(LOOKED_AT, System.currentTimeMillis()).apply();
        if (!isNewer(ready, current)) {
            ready = downloadLatest(file, current, ready);
            if (ready.isEmpty()) {
                return "";
            }
            kept.edit().putString(READY_VERSION, ready).apply();
        }
        // A look the person asked for asks again even when the same version was turned down lately.
        if (!forced && !mayAsk(kept, ready)) {
            return "";
        }
        kept.edit().putString(ASKED_VERSION, ready).putLong(ASKED_AT, System.currentTimeMillis()).apply();
        activity.runOnUiThread(() -> install(activity, kept, file));
        return ready;
    }

    /** Whether a look of its own has anything to do: a newer release already downloaded, or no look in ten minutes. */
    private static boolean worthLooking(SharedPreferences kept, String ready, String current, File file) {
        return (file.isFile() && isNewer(ready, current))
                || System.currentTimeMillis() - kept.getLong(LOOKED_AT, 0) >= LOOK_EVERY_MS;
    }

    /** The version of the release downloaded into file, empty when there is no newer one, or none to download. */
    private static String downloadLatest(File file, String current, String ready) throws Exception {
        JSONObject release = new JSONObject(read(RELEASES));
        String tag = release.optString("tag_name");
        if (!isNewer(tag, current)) {
            return "";
        }
        if (tag.equals(ready) && file.isFile()) {
            return tag;
        }
        JSONArray assets = release.getJSONArray("assets");
        String apk = addressOf(assets, APK);
        String checksum = addressOf(assets, CHECKSUM);
        if (apk.isEmpty() || checksum.isEmpty()) {
            throw new IOException("release " + tag + " has no app to install");
        }
        if (!file.getParentFile().isDirectory() && !file.getParentFile().mkdirs()) {
            throw new IOException("the folder the app is downloaded into could not be made");
        }
        download(apk, file);
        if (!sha256(file).equals(firstWord(read(checksum)).toLowerCase(Locale.ROOT))) {
            if (!file.delete()) {
                Log.w(TAG, "a download that did not match is left behind");
            }
            throw new IOException("the app of " + tag + " does not match its checksum");
        }
        return tag;
    }

    /** Whether the person may be asked again: not about the same version within a day. */
    private static boolean mayAsk(SharedPreferences kept, String ready) {
        if (!ready.equals(kept.getString(ASKED_VERSION, ""))) {
            return true;
        }
        return System.currentTimeMillis() - kept.getLong(ASKED_AT, 0) >= ASK_AGAIN_MS;
    }

    private static void install(Activity activity, SharedPreferences kept, File file) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                && !activity.getPackageManager().canRequestPackageInstalls()) {
            // Answered, Android's permission screen has the app ask again rather than a day later: it was not asked.
            kept.edit().remove(ASKED_VERSION).apply();
            activity.startActivity(
                    new Intent(
                            Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                            Uri.parse("package:" + activity.getPackageName())));
            return;
        }
        Uri address = FileProvider.getUriForFile(activity, activity.getPackageName() + ".fileprovider", file);
        Intent installer = new Intent(Intent.ACTION_VIEW)
                .setDataAndType(address, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK);
        activity.startActivity(installer);
    }

    private static String problemOf(Exception failure) {
        String said = failure.getMessage();
        return said == null || said.isEmpty() ? failure.toString() : said;
    }

    private static String versionOf(Activity activity) {
        try {
            String version = activity.getPackageManager().getPackageInfo(activity.getPackageName(), 0).versionName;
            return version == null ? "" : version;
        } catch (PackageManager.NameNotFoundException missing) {
            return "";
        }
    }

    private static boolean isNewer(String version, String current) {
        int[] theirs = numbersOf(version);
        int[] ours = numbersOf(current);
        for (int part = 0; part < PARTS; part++) {
            if (theirs[part] != ours[part]) {
                return theirs[part] > ours[part];
            }
        }
        return false;
    }

    /** A version's three numbers: v0.12.1 is 0, 12 and 1; anything else counts as none. */
    private static int[] numbersOf(String version) {
        String[] parts = version.replaceFirst("^v", "").split("\\.");
        int[] numbers = new int[PARTS];
        for (int part = 0; part < PARTS && part < parts.length; part++) {
            try {
                numbers[part] = Integer.parseInt(parts[part].replaceFirst("\\D.*$", ""));
            } catch (NumberFormatException notANumber) {
                numbers[part] = 0;
            }
        }
        return numbers;
    }

    private static String addressOf(JSONArray assets, String name) {
        for (int index = 0; index < assets.length(); index++) {
            JSONObject asset = assets.optJSONObject(index);
            if (asset != null && name.equals(asset.optString("name"))) {
                return asset.optString("browser_download_url");
            }
        }
        return "";
    }

    private static String firstWord(String text) {
        String trimmed = text.trim();
        int end = 0;
        while (end < trimmed.length() && !Character.isWhitespace(trimmed.charAt(end))) {
            end++;
        }
        return trimmed.substring(0, end);
    }

    private static String read(String address) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(address).openConnection();
        connection.setInstanceFollowRedirects(true);
        connection.setConnectTimeout(TIMEOUT_MS);
        connection.setReadTimeout(TIMEOUT_MS);
        connection.setRequestProperty("User-Agent", "swcode-app");
        try {
            if (connection.getResponseCode() != HttpURLConnection.HTTP_OK) {
                throw new IOException("HTTP " + connection.getResponseCode() + " at " + address);
            }
            try (InputStream input = connection.getInputStream()) {
                return new String(readAll(input), "UTF-8");
            }
        } finally {
            connection.disconnect();
        }
    }

    private static void download(String address, File target) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(address).openConnection();
        connection.setInstanceFollowRedirects(true);
        connection.setConnectTimeout(TIMEOUT_MS);
        connection.setReadTimeout(TIMEOUT_MS);
        connection.setRequestProperty("User-Agent", "swcode-app");
        try {
            if (connection.getResponseCode() != HttpURLConnection.HTTP_OK) {
                throw new IOException("HTTP " + connection.getResponseCode() + " at " + address);
            }
            try (
                    InputStream input = new BufferedInputStream(connection.getInputStream());
                    FileOutputStream output = new FileOutputStream(target)) {
                byte[] buffer = new byte[BUFFER];
                int read;
                while ((read = input.read(buffer)) > 0) {
                    output.write(buffer, 0, read);
                }
            }
        } finally {
            connection.disconnect();
        }
    }

    private static byte[] readAll(InputStream input) throws IOException {
        java.io.ByteArrayOutputStream collected = new java.io.ByteArrayOutputStream();
        byte[] buffer = new byte[BUFFER];
        int read;
        while ((read = input.read(buffer)) > 0) {
            collected.write(buffer, 0, read);
        }
        return collected.toByteArray();
    }

    private static String sha256(File file) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (InputStream input = new BufferedInputStream(new FileInputStream(file))) {
            byte[] buffer = new byte[BUFFER];
            int read;
            while ((read = input.read(buffer)) > 0) {
                digest.update(buffer, 0, read);
            }
        }
        StringBuilder text = new StringBuilder();
        for (byte value : digest.digest()) {
            text.append(String.format(Locale.ROOT, "%02x", value));
        }
        return text.toString();
    }
}
