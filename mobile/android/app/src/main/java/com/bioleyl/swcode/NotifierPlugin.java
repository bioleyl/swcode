package com.bioleyl.swcode;

import android.app.NotificationChannel;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.os.Build;

import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;

/**
 * The phone's own notifications, for the app whose WebView has no push service: the page tells it what a session
 * just said, Android shows it, and a tap on the notice opens that session. Android asks its permission once, and
 * the app tells again until the person changes it in Android's settings.
 */
@CapacitorPlugin(
    name = "Notifier",
    permissions = { @Permission(alias = "notifications", string = "android.permission.POST_NOTIFICATIONS") }
)
public class NotifierPlugin extends Plugin {
    /** The session a tapped notification opens, on the intent that brings the app back to the front. */
    public static final String SESSION = "swcode.session";

    private static final String CHANNEL = "swcode";
    private static final int NO_SESSION = 0;

    @PluginMethod
    public void show(PluginCall call) {
        Context context = getActivity();
        String title = call.getString("title", "swcode");
        String body = call.getString("body", "");
        NotificationManagerCompat manager = NotificationManagerCompat.from(context);
        createChannel(manager, context);

        Intent intent = new Intent(context, MainActivity.class);
        intent.putExtra(SESSION, call.getString("sessionId", ""));
        intent.setFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        int flags = Build.VERSION.SDK_INT >= 23
            ? PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
            : PendingIntent.FLAG_UPDATE_CURRENT;
        PendingIntent open = PendingIntent.getActivity(context, NO_SESSION, intent, flags);

        NotificationCompat.Builder notice = new NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(context.getApplicationInfo().icon)
            .setContentTitle(title)
            .setContentText(body)
            .setStyle(new NotificationCompat.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setContentIntent(open);
        // One notice per session, the newest winning, as the web app's service worker does with its tag.
        manager.notify(call.getString("sessionId", "").hashCode(), notice.build());
        call.resolve();
    }

    /** What a tap left behind: the session to open, empty once the page took it. */
    @PluginMethod
    public void takeSession(PluginCall call) {
        Intent intent = getActivity().getIntent();
        String sessionId = intent == null ? "" : intent.getStringExtra(SESSION);
        JSObject answer = new JSObject();
        answer.put("sessionId", sessionId == null ? "" : sessionId);
        if (sessionId != null && !sessionId.isEmpty()) {
            intent.removeExtra(SESSION);
        }
        call.resolve(answer);
    }

    private void createChannel(NotificationManagerCompat manager, Context context) {
        if (Build.VERSION.SDK_INT < 26) {
            return;
        }
        if (manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(new NotificationChannel(CHANNEL, "swcode", NotificationManagerCompat.IMPORTANCE_DEFAULT));
        }
    }
}
