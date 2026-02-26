import { enqueue } from './batch';

let isCapturing = false;
let lastPageviewUrl = '';

// Tags that represent meaningful interaction targets.
const MEANINGFUL_TAGS = new Set(['a', 'button', 'select', 'label', 'summary']);
// Input types worth recording as distinct click targets.
const MEANINGFUL_INPUT_TYPES = new Set(['submit', 'button', 'reset', 'checkbox', 'radio']);

export function startAutocapture(): void {
  if (isCapturing || typeof document === 'undefined') return;
  isCapturing = true;

  // Capture clicks via event delegation.
  document.addEventListener('click', handleClick, { capture: true, passive: true });

  // Capture form submissions.
  document.addEventListener('submit', handleSubmit, { capture: true, passive: true });

  // Capture JS errors.
  window.addEventListener('error', handleError);
  window.addEventListener('unhandledrejection', handleRejection);

  // Capture pageviews on navigation.
  capturePageview();
  window.addEventListener('popstate', capturePageview);

  // Intercept pushState/replaceState for SPA routing.
  const origPush = history.pushState;
  history.pushState = function (...args) {
    origPush.apply(this, args);
    capturePageview();
  };
  const origReplace = history.replaceState;
  history.replaceState = function (...args) {
    origReplace.apply(this, args);
    capturePageview();
  };
}

export function stopAutocapture(): void {
  if (!isCapturing) return;
  isCapturing = false;
  document.removeEventListener('click', handleClick, { capture: true });
  document.removeEventListener('submit', handleSubmit, { capture: true });
  window.removeEventListener('error', handleError);
  window.removeEventListener('unhandledrejection', handleRejection);
  window.removeEventListener('popstate', capturePageview);
}

function handleClick(e: MouseEvent): void {
  const raw = e.target as Element;
  if (!raw || !raw.tagName) return;

  // Walk up to the nearest meaningful element (button, link, etc.)
  // so clicks on icon/svg children are attributed to the right target.
  const el = getMeaningfulTarget(raw);
  const tag = el.tagName.toLowerCase();
  if (tag === 'html' || tag === 'body') return;

  enqueue({
    event_type: 'click',
    ...extractContext(el),
    timestamp: Date.now(),
    properties: {
      client_x: e.clientX / window.innerWidth,
      client_y: e.clientY / window.innerHeight,
    },
  });
}

function handleError(e: ErrorEvent): void {
  enqueue({
    event_type: 'error',
    url: window.location.href,
    url_path: window.location.pathname,
    page_title: document.title,
    timestamp: Date.now(),
    properties: {
      message: e.message,
      source: e.filename,
      lineno: e.lineno,
      colno: e.colno,
      stack: (e.error as Error | null)?.stack,
    },
  });
}

function handleRejection(e: PromiseRejectionEvent): void {
  const msg = e.reason instanceof Error ? e.reason.message : String(e.reason);
  enqueue({
    event_type: 'error',
    url: window.location.href,
    url_path: window.location.pathname,
    page_title: document.title,
    timestamp: Date.now(),
    properties: {
      message: msg,
      stack: (e.reason as Error | null)?.stack,
      type: 'unhandledrejection',
    },
  });
}

function handleSubmit(e: SubmitEvent): void {
  const form = e.target as HTMLFormElement;
  if (!form) return;

  enqueue({
    event_type: 'submit',
    ...extractContext(form),
    timestamp: Date.now(),
  });
}

function capturePageview(): void {
  // Small delay to let the URL update after navigation.
  setTimeout(() => {
    // Deduplicate: only fire if the URL actually changed.
    // Prevents duplicate pageviews from frameworks calling replaceState
    // for scroll restoration or other non-navigation purposes.
    const url = window.location.pathname + window.location.search + window.location.hash;
    if (url === lastPageviewUrl) return;
    lastPageviewUrl = url;

    const utms = getUtmParams();

    enqueue({
      event_type: 'pageview',
      url: window.location.href,
      url_path: window.location.pathname,
      page_title: document.title,
      referrer: document.referrer,
      screen_width: window.innerWidth,
      screen_height: window.innerHeight,
      timestamp: Date.now(),
      properties: Object.keys(utms).length > 0 ? utms : undefined,
    });
  }, 50);
}

// Walk up the DOM from the raw click target to find the nearest element
// that represents a meaningful interaction (link, button, form control).
// Stops at document.body or after 8 levels.
function getMeaningfulTarget(el: Element): Element {
  let current: Element | null = el;
  for (let i = 0; i < 8 && current && current !== document.body; i++) {
    const tag = current.tagName.toLowerCase();
    if (MEANINGFUL_TAGS.has(tag)) return current;
    if (tag === 'input') {
      const type = ((current as HTMLInputElement).type ?? '').toLowerCase();
      if (MEANINGFUL_INPUT_TYPES.has(type)) return current;
    }
    const role = current.getAttribute('role') ?? '';
    if (role === 'button' || role === 'link' || role === 'tab' || role === 'menuitem') return current;
    current = current.parentElement;
  }
  return el;
}

// Extract utm_* query params from the current URL.
function getUtmParams(): Record<string, string> {
  const params = new URLSearchParams(window.location.search);
  const utms: Record<string, string> = {};
  for (const key of ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term']) {
    const val = params.get(key);
    if (val) utms[key] = val;
  }
  return utms;
}

function extractContext(el: Element): Record<string, unknown> {
  const classes = Array.from(el.classList).join(' ');
  const text = (el as HTMLElement).innerText?.substring(0, 200)?.trim() ?? '';
  const dataAttrs: Record<string, string> = {};

  for (const attr of Array.from(el.attributes)) {
    if (attr.name.startsWith('data-')) {
      dataAttrs[attr.name.replace('data-', '')] = attr.value;
    }
  }

  return {
    element_tag: el.tagName.toLowerCase(),
    element_id: el.id || '',
    element_classes: classes,
    element_text: text,
    aria_label: el.getAttribute('aria-label') ?? '',
    data_attributes: Object.keys(dataAttrs).length > 0 ? dataAttrs : undefined,
    parent_path: getParentPath(el),
    url: window.location.href,
    url_path: window.location.pathname,
    page_title: document.title,
    referrer: document.referrer,
    screen_width: window.innerWidth,
    screen_height: window.innerHeight,
  };
}

function getParentPath(el: Element, maxDepth = 5): string {
  const parts: string[] = [];
  let current: Element | null = el;

  for (let i = 0; i < maxDepth && current && current !== document.body; i++) {
    let selector = current.tagName.toLowerCase();
    if (current.id) {
      selector += `#${current.id}`;
    } else if (current.classList.length > 0) {
      selector += `.${Array.from(current.classList).slice(0, 2).join('.')}`;
    }
    parts.unshift(selector);
    current = current.parentElement;
  }

  return parts.join(' > ');
}
