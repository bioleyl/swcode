package com.bioleyl.swcode;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/**
 * The page's "Look now", in Settings → Phone: the app's own look for a newer release, asked when the person asks.
 * It answers the version Android is installing, empty when this app is the latest, or why it could not look.
 */
@CapacitorPlugin(name = "Updater")
public class UpdaterPlugin extends Plugin {
    @PluginMethod
    public void check(PluginCall call) {
        Updater.lookNow(getActivity(), new Updater.Looked() {
            @Override
            public void found(String version) {
                JSObject answer = new JSObject();
                answer.put("version", version);
                call.resolve(answer);
            }

            @Override
            public void failed(String problem) {
                call.reject(problem);
            }
        });
    }
}
