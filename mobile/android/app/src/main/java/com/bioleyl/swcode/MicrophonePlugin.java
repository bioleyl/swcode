package com.bioleyl.swcode;

import android.Manifest;

import com.getcapacitor.Plugin;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;

/**
 * Android's question about the microphone, for the composer's dictation: the WebView's speech recognition needs the
 * app to hold RECORD_AUDIO and never asks for it, so the page asks through this before it listens.
 *
 * It has no method of its own: Capacitor gives every plugin `checkPermissions` and `requestPermissions`, which
 * answer `{ microphone: "granted" | "denied" | "prompt" }` from the alias declared here.
 */
@CapacitorPlugin(
    name = "Microphone",
    permissions = { @Permission(alias = "microphone", strings = { Manifest.permission.RECORD_AUDIO }) }
)
public class MicrophonePlugin extends Plugin {}
