const SESSION_KEY = '_cn_session';
const SESSION_TIMEOUT = 30 * 60 * 1000; // 30 minutes

interface SessionData {
  id: string;
  lastActivity: number;
}

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

function getSessionData(): SessionData | null {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY);
    if (!raw) return null;
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function setSessionData(data: SessionData): void {
  try {
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(data));
  } catch {
    // sessionStorage not available
  }
}

export function getSessionId(): string {
  const existing = getSessionData();
  const now = Date.now();

  if (existing && now - existing.lastActivity < SESSION_TIMEOUT) {
    existing.lastActivity = now;
    setSessionData(existing);
    return existing.id;
  }

  const newSession: SessionData = { id: generateId(), lastActivity: now };
  setSessionData(newSession);
  return newSession.id;
}
