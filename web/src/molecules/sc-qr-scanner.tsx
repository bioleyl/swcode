import { css } from '@jadis/core';
import { JlAlert, JlButton, JlDialog } from '@syware/jlib';

import { ScElement } from '../base/sc-element';
import { backCamera, qrReader } from '../services/qr-scan';
import { BASE_STYLES } from '../styles/shared';
import styles from './sc-qr-scanner.css?inline';

import type { CodeDetector } from '../types/qr-scan.types';

// How often the picture of the camera is read for a code.
const READ_EVERY_MS = 250;
const NO_CAMERA = 'The camera could not be used: allow it for this page, or paste the link instead.';

/**
 * Reads a QR code with the device's camera, in a dialog: the code another computer's swcode shows to pair with it.
 * It reports what the first code read says, and lets the camera go.
 */
export class ScQrScanner extends ScElement {
  static readonly selector = 'sc-qr-scanner';

  readonly events = this.useEvents<{ found: string }>();

  private _stream: MediaStream | undefined;
  private _clock: ReturnType<typeof setInterval> | undefined;

  private readonly _refs = this.useRefs((ref) => ({
    cancel: ref(JlButton),
    dialog: ref(JlDialog),
    problem: ref(JlAlert),
    video: ref<HTMLVideoElement>('video'),
  }));

  templateHtml(): Node {
    return (
      <JlDialog
        heading="Scan the QR code"
        description="Point the camera at the code the other computer’s swcode shows in Settings → Phone."
        size="sm">
        <video playsInline muted />
        <JlAlert tone="danger" hidden />
        <JlButton slot="actions" label="Cancel" />
      </JlDialog>
    );
  }

  templateCss(): string {
    return css`${BASE_STYLES}${styles}`;
  }

  onConnect(): void {
    this._refs.cancel.events.register('press', () => this._refs.dialog.close());
    this._refs.dialog.events.register('close', () => this.letGo());
  }

  onDisconnect(): void {
    this.letGo();
  }

  /** Opens the camera in the dialog and reads its picture until a code shows. */
  async scan(): Promise<void> {
    const reader = qrReader();
    if (!reader) {
      return;
    }
    this.showProblem('');
    this._refs.dialog.show();
    const stream = await backCamera().catch(() => undefined);
    if (!stream) {
      this.showProblem(NO_CAMERA);
      return;
    }
    this._stream = stream;
    this._refs.video.srcObject = stream;
    // A video that cannot play shows no picture, and no code is ever read: the person cancels.
    await this._refs.video.play().catch(() => undefined);
    this._clock = setInterval(() => void this.read(reader), READ_EVERY_MS);
  }

  private async read(reader: CodeDetector): Promise<void> {
    // A picture the reader cannot read yet (the camera is starting) holds no code.
    const codes = await reader.detect(this._refs.video).catch(() => []);
    const found = codes[0]?.rawValue;
    if (found === undefined || this._clock === undefined) {
      return;
    }
    this._refs.dialog.close();
    this.events.emit('found', found);
  }

  /** The camera stops with the dialog, however it closed. */
  private letGo(): void {
    clearInterval(this._clock);
    this._clock = undefined;
    for (const track of this._stream?.getTracks() ?? []) {
      track.stop();
    }
    this._stream = undefined;
    this._refs.video.srcObject = null;
  }

  private showProblem(text: string): void {
    this._refs.problem.description.set(text);
    this._refs.problem.hidden = text === '';
  }
}

ScQrScanner.register();
