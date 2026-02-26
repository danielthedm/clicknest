let flagCache: Record<string, boolean> = {};

export async function loadFlags(host: string, apiKey: string, distinctId: string): Promise<void> {
  try {
    const resp = await fetch(
      `${host}/api/v1/flags/evaluate?distinct_id=${encodeURIComponent(distinctId)}`,
      { headers: { 'X-API-Key': apiKey } },
    );
    if (!resp.ok) return;
    const data = await resp.json();
    flagCache = data.flags ?? {};
  } catch {
    // Flags default to false on error — degrade gracefully.
  }
}

export function isEnabled(key: string): boolean {
  return flagCache[key] === true;
}
