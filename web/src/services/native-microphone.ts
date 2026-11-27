import { registerPlugin } from '@capacitor/core';

const GRANTED = 'granted';

/**
 * The Android app's own plugin (mobile/android/app/src/main/java/com/bioleyl/swcode/MicrophonePlugin.java): it has
 * only what Capacitor gives every plugin, the permissions declared for it.
 */
const Microphone = registerPlugin<{ requestPermissions(): Promise<{ microphone?: string }> }>('Microphone');

/**
 * Asks Android for the microphone, which shows its question the first time and answers by itself afterwards. An app
 * that cannot be asked counts as allowed: the recognizer then says what is wrong itself.
 */
export const microphoneAllowed = (): Promise<boolean> =>
  Microphone.requestPermissions().then(
    (answer) => answer.microphone === GRANTED,
    () => true
  );
