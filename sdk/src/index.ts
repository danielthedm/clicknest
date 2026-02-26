import { initBatch, enqueue, flush } from './batch';
import { startAutocapture, stopAutocapture } from './autocapture';
import { identify, resetIdentity, getDistinctId } from './identify';
import { getSessionId } from './session';
import { loadFlags, isEnabled } from './flags';

export interface ClickNestConfig {
  apiKey: string;
  host: string;
  autocapture?: boolean;
}

let initialized = false;

function init(config: ClickNestConfig): void {
  if (initialized) return;
  initialized = true;

  const host = config.host.replace(/\/$/, '');

  initBatch({ host, apiKey: config.apiKey });

  if (config.autocapture !== false) {
    startAutocapture();
  }

  // Load feature flags in the background — isEnabled() will return false until ready.
  loadFlags(host, config.apiKey, getDistinctId() ?? '');
}

function capture(eventType: string, properties?: Record<string, unknown>): void {
  if (!initialized) return;

  enqueue({
    event_type: eventType === 'custom' ? 'custom' : eventType,
    url: window.location.href,
    url_path: window.location.pathname,
    page_title: document.title,
    timestamp: Date.now(),
    properties,
  });
}

// Auto-initialize from script tag data attributes.
if (typeof document !== 'undefined') {
  const script = document.currentScript as HTMLScriptElement | null;
  if (script) {
    const apiKey = script.getAttribute('data-api-key');
    const host = script.getAttribute('data-host');
    if (apiKey && host) {
      init({ apiKey, host });
    }
  }
}

export default {
  init,
  capture,
  identify,
  resetIdentity,
  getDistinctId,
  getSessionId,
  isEnabled,
  flush,
  startAutocapture,
  stopAutocapture,
};
