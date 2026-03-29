import { enqueue } from './batch';

let flagCache: Record<string, boolean> = {};
let experimentVariants: Record<string, string> = {};
let exposuresSent: Set<string> = new Set();

export async function loadFlags(host: string, apiKey: string, distinctId: string): Promise<void> {
  try {
    const resp = await fetch(
      `${host}/api/v1/flags/evaluate?distinct_id=${encodeURIComponent(distinctId)}`,
      { headers: { 'X-API-Key': apiKey } },
    );
    if (!resp.ok) return;
    const data = await resp.json();
    flagCache = data.flags ?? {};
    experimentVariants = {};
    if (data.experiments) {
      for (const [key, info] of Object.entries(data.experiments as Record<string, any>)) {
        if (info && typeof info === 'object' && 'variant' in info) {
          experimentVariants[key] = info.variant;
        }
      }
    }
    exposuresSent = new Set();
  } catch {
    // Flags default to false on error — degrade gracefully.
  }
}

export function isEnabled(key: string): boolean {
  const value = flagCache[key] === true;

  // Fire exposure event once per flag per session.
  if (!exposuresSent.has(key)) {
    exposuresSent.add(key);
    enqueue({
      event_type: '$exposure',
      url: typeof window !== 'undefined' ? window.location.href : '',
      url_path: typeof window !== 'undefined' ? window.location.pathname : '',
      page_title: typeof document !== 'undefined' ? document.title : '',
      timestamp: Date.now(),
      properties: {
        $flag_key: key,
        $variant: experimentVariants[key] ?? (value ? 'on' : 'off'),
      },
    });
  }

  return value;
}
