# @clicknest/sdk

The official JavaScript SDK for [ClickNest](https://github.com/danielthedm/clicknest) — self-hosted, AI-native web analytics.

## Installation

```bash
npm install @clicknest/sdk
```

Or via script tag:

```html
<script src="https://your-clicknest-host/sdk.js"
  data-api-key="cn_your_api_key"
  data-host="https://your-clicknest-host">
</script>
```

## Usage

```ts
import ClickNest from '@clicknest/sdk';

ClickNest.init({
  apiKey: 'cn_your_api_key',
  host: 'https://your-clicknest-host',
});

// Identify a user
ClickNest.identify('user-123', { email: 'user@example.com' });

// Track a custom event
ClickNest.track('Signed up');

// Feature flags
const enabled = ClickNest.isEnabled('my-feature');
```

Pageviews and clicks are captured automatically — no manual instrumentation needed.

## Self-hosting

See the [ClickNest README](https://github.com/danielthedm/clicknest) for deployment instructions.
