import type { CodeDetector } from '../types/qr-scan.types';

const QR_FORMAT = 'qr_code';

/**
 * Whether this device reads a QR code with its camera from the page: its browser reads codes, and lets the page
 * use the camera, which it does over HTTPS or on the computer itself.
 */
export const canScan = (): boolean =>
  window.isSecureContext && window.BarcodeDetector !== undefined && navigator.mediaDevices !== undefined;

/** The camera at the back of the device, for a video to show. */
export const backCamera = (): Promise<MediaStream> =>
  navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } });

/** The browser's reader of QR codes; none where it has no reader. */
export function qrReader(): CodeDetector | undefined {
  const Detector = window.BarcodeDetector;
  return Detector ? new Detector({ formats: [QR_FORMAT] }) : undefined;
}
