### 2. Are there more robust solutions/practices?

While timeouts are the foundation, modern systems use more sophisticated patterns to handle them gracefully:

#### A. Context Propagation (Go `context.Context`)

Instead of setting ad-hoc timeouts at individual boundaries, modern Go services propagate a `context.Context` initialized with a deadline (`context.WithTimeout`).

* **Why it's robust:** If the top-level request times out, the cancellation signal automatically cascades down the entire execution tree—aborting ongoing database queries, closing file readers, and terminating child processes immediately.

#### B. Reactive Backpressure (Flow Control)

Rather than timing out or dropping packets when a queue fills up, the server can stop reading from the source.

* **How Suprasole does this:** When a client’s socket cannot keep up, the workspace's ring buffer fills. The server stops draining stdout from the PTY. The OS PTY buffer then fills up, which naturally blocks the child process (`bash`/`sh`) from writing to stdout. This pushes the congestion back to the producer without requiring timeouts or dropping data.

#### C. Keep-Alives (Application-Level Heartbeats)

Timeouts are most robust when combined with active heartbeats (like the WebSocket Ping/Pong loop).

* **Why it's robust:** Instead of waiting to detect a dead connection only when we next try to send data, active heartbeats periodically probe the channel. This allows the server to discover connection losses early and begin clean session teardown.




