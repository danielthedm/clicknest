/**
 * ClickNest Node.js SDK for server-side analytics.
 */

class ClickNest {
  /**
   * @param {Object} options
   * @param {string} options.apiKey
   * @param {string} options.host
   * @param {number} [options.flushInterval=10000] - ms between flushes
   * @param {number} [options.maxBatchSize=50]
   */
  constructor({ apiKey, host, flushInterval = 10000, maxBatchSize = 50 }) {
    this.apiKey = apiKey;
    this.host = host.replace(/\/$/, "");
    this.maxBatchSize = maxBatchSize;
    this._queue = [];
    this._timer = setInterval(() => this.flush(), flushInterval);
    if (this._timer.unref) this._timer.unref();
  }

  /**
   * Track an event.
   * @param {string} event - Event name
   * @param {Object} [options]
   * @param {string} [options.distinctId]
   * @param {Object} [options.properties]
   */
  capture(event, { distinctId, properties } = {}) {
    this._queue.push({
      event: {
        event_type: "custom",
        url: event,
        url_path: event,
        timestamp: Date.now(),
        properties: { ...properties, $event_name: event },
      },
      distinct_id: distinctId || "",
    });

    if (this._queue.length >= this.maxBatchSize) {
      this.flush();
    }
  }

  /**
   * Link an anonymous ID to an identified user.
   * @param {string} distinctId
   * @param {string} [previousId]
   */
  identify(distinctId, previousId) {
    if (previousId) {
      this.capture("$identify", {
        distinctId,
        properties: { previous_id: previousId },
      });
    }
  }

  /** Force flush all queued events. */
  async flush() {
    if (this._queue.length === 0) return;

    const items = this._queue.splice(0);

    // Group by distinct_id.
    const batches = {};
    for (const item of items) {
      const did = item.distinct_id;
      if (!batches[did]) batches[did] = [];
      batches[did].push(item.event);
    }

    const promises = Object.entries(batches).map(([did, events]) => {
      const body = JSON.stringify({
        events,
        session_id: `server-${Date.now()}`,
        distinct_id: did,
      });

      return fetch(`${this.host}/api/v1/events`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-API-Key": this.apiKey,
        },
        body,
      }).catch(() => {}); // Best effort.
    });

    await Promise.all(promises);
  }

  /** Flush and stop the timer. */
  async shutdown() {
    clearInterval(this._timer);
    await this.flush();
  }
}

module.exports = { ClickNest };
