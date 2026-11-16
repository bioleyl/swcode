/**
 * Whether the page runs in swcode's Android app, which brings its own pages and pairs with a computer: an install
 * button and web notifications mean nothing there, and a WebView has no push service.
 */
export const inAndroidApp = (): boolean => window.Capacitor?.isNativePlatform?.() === true;
