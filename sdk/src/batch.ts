import { send, TransportConfig, TransportPayload } from './transport';
import { getSessionId } from './session';
import { getDistinctId } from './identify';

const MAX_BATCH_SIZE = 10;
const FLUSH_INTERVAL = 5000; // 5 seconds

let queue: Record<string, unknown>[] = [];
let flushTimer: ReturnType<typeof setTimeout> | null = null;
let config: TransportConfig | null = null;

export function initBatch(transportConfig: TransportConfig): void {
  config = transportConfig;
  startFlushTimer();

  // Flush on page unload.
  if (typeof window !== 'undefined') {
    window.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'hidden') {
        flush();
      }
    });
    window.addEventListener('pagehide', flush);
  }
}

export function enqueue(event: Record<string, unknown>): void {
  queue.push(event);

  if (queue.length >= MAX_BATCH_SIZE) {
    flush();
  }
}

export function flush(): void {
  if (!config || queue.length === 0) return;

  const events = queue.splice(0);
  const payload: TransportPayload = {
    events,
    session_id: getSessionId(),
    distinct_id: getDistinctId(),
  };

  send(config, payload);
}

function startFlushTimer(): void {
  if (flushTimer) clearInterval(flushTimer);
  flushTimer = setInterval(flush, FLUSH_INTERVAL);
}
