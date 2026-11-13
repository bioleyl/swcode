/** A code the browser read in a picture: what it says. */
export interface DetectedCode {
  rawValue: string;
}

/** The browser's own reader of codes in pictures, where it has one. */
export interface CodeDetector {
  detect(source: HTMLVideoElement): Promise<DetectedCode[]>;
}

declare global {
  interface Window {
    // biome-ignore lint/style/useNamingConvention: the browser's own name for it.
    BarcodeDetector?: new (options: {
      formats: string[];
    }) => CodeDetector;
  }
}
