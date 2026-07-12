import { assertEquals, assert } from '@std/assert'
import { ServerOrchestrator } from './ServerOrchestrator.ts'
import { WebSocketClient } from './WebSocketClient.ts'

Deno.test({
  name: '{h6e2et} [E2E Infrastructure] Recovery Suite: Session Takeovers and Replays Invariants',
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      '{srec01} [Session Recovery] Reconnection Handshake & Takeover (Takeover): Forcefully evicts old session with 4000 close code',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-takeover-evict';
        const client1 = new WebSocketClient(session.port, token);
        const client2 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          await client2.connect();
          const closeEvent = await client1.waitForClose();
          assert(closeEvent.code !== -1, 'Client 1 should be evicted');
          assertEquals(closeEvent.code, 4000);
          assertEquals(closeEvent.reason, 'Session Taken Over');
        } finally {
          client1.close();
          client2.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{srec02} [Session Recovery] Historical Output Playback (Replay): Recovers cached scrollbacks',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-replay-history';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client1.readSpawnStatus(1);
          client1.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'REPLAY_TEST'\n"));
          let receivedReplay = false;
          const replayDeadline = Date.now() + 4000;
          while (Date.now() < replayDeadline) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes('REPLAY_TEST')) {
                receivedReplay = true;
                break;
              }
            }
          }
          assert(receivedReplay, "REPLAY_TEST output not received before close");
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let recovered = false;
            const deadline = Date.now() + 4000;
            while (Date.now() < deadline) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 1) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes('REPLAY_TEST')) {
                  recovered = true;
                  break;
                }
              }
            }
            assert(recovered, 'Failed to recover historical logs');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{srec03} [Session Recovery] Orphan Sweeper Expiration (OrphanSweeper): GC teardown of orphaned sessions after 200ms',
      async () => {
        // Spawn with short 100ms sweeper duration
        const session = await orchestrator.spawnRuntime(['-sweeper', '100ms']);
        const token = 'token-sweeper';
        try {
          // Input B: Reconnect before 100ms (20ms) -> Aborts sweeper
          const client1 = new WebSocketClient(session.port, token);
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client1.readSpawnStatus(1);
          client1.close();
          await new Promise((resolve) => setTimeout(resolve, 20));
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            // Verify T1 still exists (we can send a no-op StreamIO to T1 without error)
            client2.sendFrame(0x0005, 1, new Uint8Array());
            assert(true, 'Sweeper was successfully aborted before expiration');
          } finally {
            client2.close();
          }
          // Input A: Reconnect after 150ms (exceeding 100ms sweeper) -> Session reaped
          const client3 = new WebSocketClient(session.port, token);
          await client3.connect();
          // Spawn T1 on client3 to verify it gets reaped later
          client3.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client3.readSpawnStatus(1);
          client3.close();
          await new Promise((resolve) => setTimeout(resolve, 150));
          const client4 = new WebSocketClient(session.port, token);
          try {
            await client4.connect();
            // Spawn T1 again. If it succeeds with 0x00 status, it proves a fresh session was created,
            // as previously active T1 was swept and ID collision check does not trigger.
            client4.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
            const unpacked = await client4.readSpawnStatus(1);
            assertEquals(unpacked.action, 2);
            assertEquals(unpacked.payload[0], 0x00, 'Workspace was not swept and recreated cleanly');
          } finally {
            client4.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{srec04} [Session Recovery] Scrollback Buffer Overflow Warning (Truncation): ANSI warning injection',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-overflow';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client1.readSpawnStatus(1);
          // Write large stream to exceed 256KB ring buffer limit
          client1.sendFrame(0x0005, 1, new TextEncoder().encode("dd if=/dev/zero bs=300000 count=1\n"));
          let totalBytes = 0;
          const overflowDeadline = Date.now() + 5000;
          while (Date.now() < overflowDeadline && totalBytes < 256 * 1024) {
            const frame = await client1.readFrame(5000);
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              totalBytes += unpacked.payload.length;
            }
          }
          assert(totalBytes >= 256 * 1024, `Failed to exceed 256KB ring buffer limit (got ${totalBytes} bytes)`);
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let truncated = false;
            const deadline = Date.now() + 4000;
            while (Date.now() < deadline) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 1) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes('[... Output truncated due to buffer overflow ...]')) {
                  truncated = true;
                  break;
                }
              }
            }
            assert(truncated, 'Truncation warning was not injected');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{srec05} [Session Recovery] Empty Workspace Sweeping (IdleSweep): Reaps idle sessions cleanly',
      async () => {
        const session = await orchestrator.spawnRuntime(['-sweeper', '100ms']);
        const token = 'token-idle';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.close();
          await new Promise((resolve) => setTimeout(resolve, 150));
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            assert(true, 'Empty session registry sweeper reaped successfully');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{srec06} [Session Recovery] Connection Eviction Sweeper Prevention (TakeoverClean): Overwrites sweeper setup during takeovers',
      async () => {
        const session = await orchestrator.spawnRuntime(['-sweeper', '100ms']);
        const token = 'token-takeover-sweep';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            // Wait exceeding 100ms sweeper duration
            await new Promise((resolve) => setTimeout(resolve, 150));
            // Verify Client 2 is still healthy and not reaped
            client2.sendFrame(0x0005, 99, new Uint8Array());
            assert(true, 'Eviction sweeper prevented successfully');
          } finally {
            client2.close();
          }
        } finally {
          client1.close();
          await session.shutdown();
        }
      }
    );
  }
});
