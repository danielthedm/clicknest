export interface TransportConfig {
  host: string;
  apiKey: string;
}

export interface TransportPayload {
  events: Record<string, unknown>[];
  session_id: string;
  distinct_id: string | null;
}

const MAX_RETRIES = 3;
const RETRY_DELAYS = [1000, 5000, 15000];

export async function send(
  config: TransportConfig,
  payload: TransportPayload,
  retryCount = 0
): Promise<boolean> {
  const url = `${config.host.replace(/\/$/, '')}/api/v1/events`;

  try {
    const resp = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': config.apiKey,
      },
      body: JSON.stringify(payload),
      keepalive: true,
    });

    if (resp.ok) return true;

    // Retry on 5xx errors.
    if (resp.status >= 500 && retryCount < MAX_RETRIES) {
      await delay(RETRY_DELAYS[retryCount] ?? 15000);
      return send(config, payload, retryCount + 1);
    }

    return false;
  } catch {
    if (retryCount < MAX_RETRIES) {
      await delay(RETRY_DELAYS[retryCount] ?? 15000);
      return send(config, payload, retryCount + 1);
    }
    return false;
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
