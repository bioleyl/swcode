package com.bioleyl.swcode;

import android.app.NotificationChannel;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;
import android.util.Log;

import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import androidx.core.app.ServiceCompat;

/**
 * The app's notice that it keeps watching its computer: a service at the front of Android's list, so the process is
 * not frozen and the daemon's socket keeps speaking while the phone sleeps. Its own notice says so, and a tap on it
 * opens the app.
 */
public class NotifierService extends Service {
    /** What Android keeps while the service runs, so the app knows it is up without a binder. */
    private static volatile NotifierService running;

    private static final int ONGOING = 1;
    private static final String CHANNEL = "swcode-background";
    private static final String TAG = "swcode";

    /** Whether the phone is kept awake for swcode's sessions, which the settings' button shows. */
    public static boolean isRunning() {
        return running != null;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        running = this;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        Context context = this;
        NotificationManagerCompat manager = NotificationManagerCompat.from(context);
        createChannel(manager, context);
        Intent open = new Intent(context, MainActivity.class);
        PendingIntent tap = PendingIntent.getActivity(
            context,
            0,
            open,
            Build.VERSION.SDK_INT >= 23
                ? PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
                : PendingIntent.FLAG_UPDATE_CURRENT
        );
        NotificationCompat.Builder notice = new NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(context.getApplicationInfo().icon)
            .setContentTitle("swcode")
            .setContentText("Watches your sessions on this computer")
            .setContentIntent(tap)
            .setOngoing(true);
        // The type tells Android the socket goes to a device of the network, which is what keeps it running. A device
        // that refuses it anyway (an Android version whose rules the manifest does not meet, an OEM's own) throws
        // here, and an uncaught throw would take the app down as it opens: the notices go, the app stays.
        try {
            ServiceCompat.startForeground(this, ONGOING, notice.build(), ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE);
        } catch (RuntimeException refused) {
            Log.w(TAG, "Android did not take the notice service to its front", refused);
            stopSelf();
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        running = null;
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void createChannel(NotificationManagerCompat manager, Context context) {
        if (Build.VERSION.SDK_INT < 26) {
            return;
        }
        if (manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(
                new NotificationChannel(CHANNEL, "swcode", NotificationManagerCompat.IMPORTANCE_LOW));
        }
    }
}
