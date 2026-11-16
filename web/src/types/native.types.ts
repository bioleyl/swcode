/** Capacitor's bridge in the app a phone installed, which it puts on `window.Capacitor`. */
export interface CapacitorBridge {
  isNativePlatform?: () => boolean;
}

declare global {
  interface Window {
    // biome-ignore lint/style/useNamingConvention: Capacitor puts its bridge under this name.
    Capacitor?: CapacitorBridge;
  }
}
