package com.bioleyl.swcode;

import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle state) {
        super.onCreate(state);
        // The app comes from a release, not from a store: it takes the next one itself.
        Updater.check(this);
    }
}
