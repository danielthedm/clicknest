"""ClickNest Python SDK for server-side analytics."""

import atexit
import json
import threading
import time
import urllib.request
from typing import Any, Optional


class ClickNest:
    """ClickNest analytics client for Python backends."""

    def __init__(
        self,
        api_key: str,
        host: str,
        flush_interval: float = 10.0,
        max_batch_size: int = 50,
    ):
        self.api_key = api_key
        self.host = host.rstrip("/")
        self.flush_interval = flush_interval
        self.max_batch_size = max_batch_size
        self._queue: list[dict] = []
        self._lock = threading.Lock()
        self._running = True

        self._timer = threading.Thread(target=self._flush_loop, daemon=True)
        self._timer.start()
        atexit.register(self.shutdown)

    def capture(
        self,
        event: str,
        distinct_id: Optional[str] = None,
        properties: Optional[dict[str, Any]] = None,
    ) -> None:
        """Track an event."""
        payload = {
            "event_type": "custom",
            "url": event,
            "url_path": event,
            "timestamp": int(time.time() * 1000),
            "properties": {**(properties or {}), "$event_name": event},
        }
        with self._lock:
            self._queue.append(
                {
                    "event": payload,
                    "distinct_id": distinct_id or "",
                }
            )
            if len(self._queue) >= self.max_batch_size:
                self._flush()

    def identify(self, distinct_id: str, previous_id: Optional[str] = None) -> None:
        """Link an anonymous ID to an identified user."""
        if previous_id:
            self.capture(
                "$identify",
                distinct_id=distinct_id,
                properties={"previous_id": previous_id},
            )

    def flush(self) -> None:
        """Force flush all queued events."""
        with self._lock:
            self._flush()

    def shutdown(self) -> None:
        """Flush remaining events and stop the background thread."""
        self._running = False
        self.flush()

    def _flush(self) -> None:
        if not self._queue:
            return

        # Group events by distinct_id for batching.
        batches: dict[str, list[dict]] = {}
        for item in self._queue:
            did = item["distinct_id"]
            batches.setdefault(did, []).append(item["event"])
        self._queue = []

        for did, events in batches.items():
            body = json.dumps(
                {
                    "events": events,
                    "session_id": f"server-{int(time.time())}",
                    "distinct_id": did,
                }
            ).encode()

            req = urllib.request.Request(
                f"{self.host}/api/v1/events",
                data=body,
                headers={
                    "Content-Type": "application/json",
                    "X-API-Key": self.api_key,
                },
            )
            try:
                urllib.request.urlopen(req, timeout=5)
            except Exception:
                pass  # Best effort — don't crash the app.

    def _flush_loop(self) -> None:
        while self._running:
            time.sleep(self.flush_interval)
            with self._lock:
                self._flush()
