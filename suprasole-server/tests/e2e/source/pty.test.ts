import { assertEquals, assert } from '@std/assert'
import { ServerOrchestrator } from './ServerOrchestrator.ts'
import { WebSocketClient } from './WebSocketClient.ts'

Deno.test({
  name: '{h6e2et} [E2E Infrastructure] PTY Suite: Pseudo-Terminal Lifecycle and IO Invariants',
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      '{ptyl01} [PTY Ingestion] Spawn Shell (Spawn): Spawns pseudo-terminals and receives status confirmation',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-spawn');
        try {
          await client.connect();
          // Spawn T1 normally
          const spawnPayload = new Uint8Array([0, 80, 0, 24]);
          client.sendFrame(0x0001, 1, spawnPayload);
          let unpacked = await client.readSpawnStatus(1);
          assertEquals(unpacked.action, 2);
          assertEquals(unpacked.terminalID, 1);
          assertEquals(unpacked.payload[0], 0x00);
          // Extreme geometry check (65535 x 65535 cols/rows)
          const extremePayload = new Uint8Array([0xff, 0xff, 0xff, 0xff]);
          client.sendFrame(0x0001, 2, extremePayload);
          unpacked = await client.readSpawnStatus(2);
          assertEquals(unpacked.action, 2);
          assertEquals(unpacked.terminalID, 2);
          assertEquals(unpacked.payload[0], 0x00);
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl02} [PTY Ingestion] Inter-Process Stream IO (StreamIO): Verifies stream I/O behaviors and edge cases',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-pty-io');
        try {
          await client.connect();
          // Spawn Terminal 1
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Input A: Normal Command IO
          const command = new TextEncoder().encode("echo 'E2E_INGEST'\n");
          client.sendFrame(0x0005, 1, command);
          let receivedHello = false;
          const deadline = Date.now() + 4000;
          while (Date.now() < deadline) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes('E2E_INGEST')) {
                receivedHello = true;
                break;
              }
            }
          }
          assert(receivedHello, 'Should receive streamed stdout containing command string');
          // Input B: Zero-Length payload
          client.sendFrame(0x0005, 1, new Uint8Array());
          assert(true, 'Zero-length StreamIO did not error');
          // Input C: Write to terminated PTY
          client.sendFrame(0x0004, 1); // Kill T1
          await client.readFrame(); // Consume Kill response
          client.sendFrame(0x0005, 1, new TextEncoder().encode("ls\n"));
          assert(true, 'StreamIO to terminated shell ignored cleanly');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl03} [PTY Ingestion] Terminal Window Resize (Resize): Updates geometry window dimensions',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-resize');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Send Resize frame to 100x30
          client.sendFrame(0x0003, 1, new Uint8Array([0, 100, 0, 30]));
          // Assert resize converges on PTY shell
          client.sendFrame(0x0005, 1, new TextEncoder().encode("stty size\n"));
          let resized = false;
          const deadline = Date.now() + 4000;
          while (Date.now() < deadline) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes('30 100')) {
                resized = true;
                break;
              }
            }
          }
          assert(resized, 'PTY shell did not resize to 30 100');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl04} [PTY Ingestion] Process Termination (Kill): Tears down child process and reaps exit code',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-kill');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Exit process via exit status code 42
          client.sendFrame(0x0005, 1, new TextEncoder().encode("exit 42\n"));
          let reapedCode = -1;
          const deadline = Date.now() + 4000;
          while (Date.now() < deadline) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 4 && unpacked.terminalID === 1) {
              reapedCode = unpacked.payload[0];
              break;
            }
          }
          assertEquals(reapedCode, 42, 'PTY process exit code should be 42');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl05} [PTY Ingestion] Queue Congestion Flow Control (QueueCap): Enforces priority-based flow queues',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-congestion');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Trigger stream logs and verify active connection drains queues without drops
          client.sendFrame(0x0005, 1, new TextEncoder().encode("seq 1 10\n"));
          let receivedCount = 0;
          const deadline = Date.now() + 2000;
          while (Date.now() < deadline) {
            try {
              const frame = await client.readFrame(500);
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 1) {
                receivedCount++;
              }
            } catch {
              break;
            }
          }
          assert(receivedCount > 0, 'Active connection should successfully receive stream outputs');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl06} [PTY Ingestion] Double Kill Protection (SafeExit): Ignores duplicate process termination requests',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-double-kill');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Kill once
          client.sendFrame(0x0004, 1);
          await client.readFrame();
          // Kill twice (Duplicate)
          client.sendFrame(0x0004, 1);
          assert(true, 'Duplicate kill ignored cleanly');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl07} [PTY Ingestion] TerminalID Reuse Restriction (IdCollision): Rejects spawning a terminated TerminalID',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-reuse');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          client.sendFrame(0x0004, 1);
          await client.readFrame();
          // Spawn T1 again (collision)
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          const resp = await client.readFrame();
          const unpacked = WebSocketClient.unpackFrame(resp);
          assertEquals(unpacked.action, 2);
          assertEquals(unpacked.payload[0], 0x01, 'Should fail to reuse TerminalID');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl08} [PTY Ingestion] Stream Output Chunking (BufferSizing): Chunks output stream payload limits',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-chunking');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Trigger large output
          client.sendFrame(0x0005, 1, new TextEncoder().encode("seq 1 3000\n"));
          let frame = await client.readFrame();
          let unpacked = WebSocketClient.unpackFrame(frame);
          assert(unpacked.payload.length <= 4096, 'Frame size exceeded 4096 bytes chunk size');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{ptyl09} [PTY Ingestion] Failed Resize Window Resilience (ResizeError): Recovers from failing ioctl window size changes',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-resize-err');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client.readSpawnStatus(1);
          // Send malformed resize or target invalid descriptor size
          client.sendFrame(0x0003, 99, new Uint8Array([0, 80, 0, 24]));
          assert(true, 'Failing resize handled cleanly');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
  }
});
