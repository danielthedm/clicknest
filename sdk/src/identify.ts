import { enqueue } from './batch';

const DISTINCT_KEY = '_cn_distinct';

let currentDistinctId: string | null = null;

function generateId(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let id = '';
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  for (const byte of array) {
    id += chars[byte % chars.length];
  }
  return id;
}

export function getDistinctId(): string {
  if (currentDistinctId) return currentDistinctId;
  try {
    const stored = localStorage.getItem(DISTINCT_KEY);
    if (stored) return stored;
    // Auto-generate an anonymous ID so every visitor is tracked.
    const anonId = generateId();
    localStorage.setItem(DISTINCT_KEY, anonId);
    return anonId;
  } catch {
    // localStorage not available — return a transient ID.
    const transient = generateId();
    currentDistinctId = transient;
    return transient;
  }
}

export function identify(distinctId: string): void {
  const previousId = getDistinctId();

  currentDistinctId = distinctId;
  try {
    localStorage.setItem(DISTINCT_KEY, distinctId);
  } catch {
    // localStorage not available
  }

  // Emit a $identify event so the backend can link the anonymous ID
  // to the newly identified user and backfill past events.
  if (previousId && previousId !== distinctId) {
    enqueue({
      event_type: '$identify',
      timestamp: Date.now(),
      url: typeof window !== 'undefined' ? window.location.href : '',
      url_path: typeof window !== 'undefined' ? window.location.pathname : '',
      properties: {
        previous_id: previousId,
      },
    });
  }
}

export function resetIdentity(): void {
  currentDistinctId = null;
  try {
    localStorage.removeItem(DISTINCT_KEY);
  } catch {
    // localStorage not available
  }
}
