export class WebSocketClient {
  private wsUrl: string;
  private ws: WebSocket | null = null;
  private messageQueue: Uint8Array[] = [];
  private resolveQueue: { resolve: (value: Uint8Array) => void; reject: (err: Error) => void; timer: any }[] = [];
  private closeEvent: { code: number; reason: string } | null = null;
  constructor(port: number, token: string) {
    this.wsUrl = `ws://127.0.0.1:${port}/ws?token=${token}`;
  }
  async connect(): Promise<void> {
    this.ws = new WebSocket(this.wsUrl);
    this.ws.binaryType = 'arraybuffer';
    return new Promise((resolve, reject) => {
      this.ws!.onopen = () => resolve();
      this.ws!.onerror = (err) => reject(err);
      this.ws!.onmessage = (event) => {
        const data = new Uint8Array(event.data as ArrayBuffer);
        if (this.resolveQueue.length > 0) {
          const next = this.resolveQueue.shift()!;
          clearTimeout(next.timer);
          next.resolve(data);
        } else {
          this.messageQueue.push(data);
        }
      };
      this.ws!.onclose = (event) => {
        this.closeEvent = { code: event.code, reason: event.reason };
        this.cleanupPending(new Error('WebSocket connection closed'));
      };
    });
  }
  private cleanupPending(err: Error): void {
    const pending = this.resolveQueue;
    this.resolveQueue = [];
    for (const item of pending) {
      clearTimeout(item.timer);
      item.reject(err);
    }
  }
  sendFrame(action: number, terminalID: number, payload: Uint8Array = new Uint8Array()): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not open');
    }
    const frame = new Uint8Array(4 + payload.length);
    frame[0] = (action >> 8) & 0xff;
    frame[1] = action & 0xff;
    frame[2] = (terminalID >> 8) & 0xff;
    frame[3] = terminalID & 0xff;
    frame.set(payload, 4);
    this.ws.send(frame);
  }
  async readFrame(timeoutMs: number = 200): Promise<Uint8Array> {
    if (this.messageQueue.length > 0) {
      return this.messageQueue.shift()!;
    }
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not open');
    }
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        const idx = this.resolveQueue.findIndex((item) => item.timer === timer);
        if (idx !== -1) {
          this.resolveQueue.splice(idx, 1);
        }
        reject(new Error('timeout waiting for WebSocket frame'));
      }, timeoutMs);
      this.resolveQueue.push({ resolve, reject, timer });
    });
  }
  async readSpawnStatus(terminalID: number, timeoutMs: number = 200): Promise<{ action: number; terminalID: number; payload: Uint8Array }> {
    const deadline = Date.now() + timeoutMs;
    const skipped: Uint8Array[] = [];
    try {
      while (Date.now() < deadline) {
        const timeoutLeft = Math.max(10, deadline - Date.now());
        const frame = await this.readFrame(timeoutLeft);
        const unpacked = WebSocketClient.unpackFrame(frame);
        if (unpacked.action === 2 && unpacked.terminalID === terminalID) {
          if (skipped.length > 0) {
            this.messageQueue.unshift(...skipped);
          }
          return unpacked;
        }
        skipped.push(frame);
      }
      throw new Error(`Spawn status for terminal ${terminalID} not received within timeout`);
    } catch (err) {
      if (skipped.length > 0) {
        this.messageQueue.unshift(...skipped);
      }
      throw err;
    }
  }
  async waitForClose(timeoutMs: number = 200): Promise<{ code: number; reason: string }> {
    if (this.closeEvent) {
      return this.closeEvent;
    }
    if (!this.ws) {
      return { code: -1, reason: 'WebSocket not initialized' };
    }
    const ws = this.ws;
    return new Promise((resolve) => {
      const originalOnClose = ws.onclose;
      const timer = setTimeout(() => {
        ws.onclose = originalOnClose;
        resolve({ code: -1, reason: 'timeout' });
      }, timeoutMs);
      ws.onclose = (event) => {
        clearTimeout(timer);
        this.closeEvent = { code: event.code, reason: event.reason };
        ws.onclose = originalOnClose;
        resolve(this.closeEvent);
      };
    });
  }
  close(): void {
    this.cleanupPending(new Error('WebSocket closed by client'));
    if (this.ws) {
      this.ws.close();
    }
  }
  static unpackFrame(frame: Uint8Array): { action: number; terminalID: number; payload: Uint8Array } {
    const action = (frame[0] << 8) | frame[1];
    const terminalID = (frame[2] << 8) | frame[3];
    const payload = frame.slice(4);
    return { action, terminalID, payload };
  }
}
