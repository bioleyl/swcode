package com.bioleyl.swcode;

import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle state) {
        // The page's "Look now" reaches the app's own update check through it.
        registerPlugin(UpdaterPlugin.class);
        super.onCreate(state);
    }

    @Override
    public void onResume() {
        super.onResume();
        // The app usually comes back rather than opening anew, so a release out since the last look is offered here.
        Updater.check(this);
    }
}
