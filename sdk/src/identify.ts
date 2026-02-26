const DISTINCT_KEY = '_cn_distinct';

let currentDistinctId: string | null = null;

export function getDistinctId(): string | null {
  if (currentDistinctId) return currentDistinctId;
  try {
    return localStorage.getItem(DISTINCT_KEY);
  } catch {
    return null;
  }
}

export function identify(distinctId: string): void {
  currentDistinctId = distinctId;
  try {
    localStorage.setItem(DISTINCT_KEY, distinctId);
  } catch {
    // localStorage not available
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
