package com.bioleyl.swcode;

import android.content.Intent;
import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle state) {
        // The page's "Look now" reaches the app's own update check through it, its notices through Notifier, and
        // Android's question about the microphone through Microphone.
        registerPlugin(UpdaterPlugin.class);
        registerPlugin(NotifierPlugin.class);
        registerPlugin(MicrophonePlugin.class);
        super.onCreate(state);
    }

    @Override
    public void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        // A notice tapped while the app ran: the session it opens rides on this intent, until the page takes it.
        setIntent(intent);
    }

    @Override
    public void onResume() {
        super.onResume();
        // The app usually comes back rather than opening anew, so a release out since the last look is offered here.
        Updater.check(this);
    }
}
