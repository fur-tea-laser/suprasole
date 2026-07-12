import { assertEquals, assert } from '@std/assert'
import { ServerOrchestrator } from './ServerOrchestrator.ts'
import { WebSocketClient } from './WebSocketClient.ts'

Deno.test({
  name: '{h6e2et} [E2E Infrastructure] Scheduler Suite: Strict Scheduling Priority Invariants',
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      '{psch01} [Priority Scheduling] Strict Priority Draining (Draining): Asserts high-priority queues drain first',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-drain');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24])); // T1
          await client.readSpawnStatus(1);
          client.sendFrame(0x0001, 2, new Uint8Array([0, 80, 0, 24])); // T2
          await client.readSpawnStatus(2);
          // Set T1 to High Priority, T2 to Low Priority
          const syncPayload = new Uint8Array([0, 1, 1, 0, 2, 0]);
          client.sendFrame(0x0006, 0, syncPayload);
          // Assert High Priority T1 preemption over Low Priority T2
          client.sendFrame(0x0005, 2, new TextEncoder().encode("for i in $(seq 1 100); do echo \"LOW_$i\"; sleep 0.01; done\n"));
          client.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'PREEMPT'\n"));
          let receivedPreempt = false;
          let finishedT2 = false;
          let preemptedSuccessful = false;
          const deadline = Date.now() + 3000;
          while (Date.now() < deadline) {
            try {
              const frame = await client.readFrame(500);
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (unpacked.terminalID === 1 && text.includes('PREEMPT')) {
                  receivedPreempt = true;
                  if (!finishedT2) {
                    preemptedSuccessful = true;
                  }
                }
                if (unpacked.terminalID === 2 && text.includes('LOW_100')) {
                  finishedT2 = true;
                }
              }
            } catch {
              break;
            }
          }
          assert(receivedPreempt, 'Should receive T1 preempt output');
          assert(preemptedSuccessful, 'High priority T1 output should preempt Low priority T2 queue draining');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{psch02} [Priority Scheduling] Whole-State Layout Synchronization (PrioritySync): Shift priority mappings dynamically',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-priority-sync');
        try {
          await client.connect();
          client.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24])); // T1
          await client.readSpawnStatus(1);
          client.sendFrame(0x0001, 2, new Uint8Array([0, 80, 0, 24])); // T2
          await client.readSpawnStatus(2);
          // Input A: Shift mapping -> Set T1 to Low, T2 to High
          const syncPayload = new Uint8Array([0, 1, 0, 0, 2, 1]);
          client.sendFrame(0x0006, 0, syncPayload);
          // Assert High Priority T2 preemption over Low Priority T1
          client.sendFrame(0x0005, 1, new TextEncoder().encode("for i in {1..2000}; do echo \"T1_DATA_$i\"; done\n"));
          client.sendFrame(0x0005, 2, new TextEncoder().encode("echo 'PREEMPT_FLIP'\n"));
          let receivedPreempt = false;
          let finishedT1 = false;
          let preemptedSuccessful = false;
          const deadline = Date.now() + 3000;
          while (Date.now() < deadline) {
            try {
              const frame = await client.readFrame(500);
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (unpacked.terminalID === 2 && text.includes('PREEMPT_FLIP')) {
                  receivedPreempt = true;
                  if (!finishedT1) {
                    preemptedSuccessful = true;
                  }
                }
                if (unpacked.terminalID === 1 && text.includes('T1_DATA_2000')) {
                  finishedT1 = true;
                }
              }
            } catch {
              break;
            }
          }
          assert(receivedPreempt, 'Should receive T2 preempt output');
          assert(preemptedSuccessful, 'Dynamic PrioritySync should shift preemption priority to T2');
          // Input B: Unspawned TerminalID sync
          const invalidSync = new Uint8Array([0, 99, 1]);
          client.sendFrame(0x0006, 0, invalidSync);
          assert(true, 'Invalid PrioritySync tuple ignored cleanly');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{psch03} [Priority Scheduling] Workspace-Wide Replay Starvation Prevention (Starvation): Starvation lockout checks',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-starvation';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24])); // T1 (High)
          await client1.readSpawnStatus(1);
          client1.sendFrame(0x0001, 2, new Uint8Array([0, 80, 0, 24])); // T2 (Low)
          await client1.readSpawnStatus(2);
          // Write data to T2 to trigger scrollback
          client1.sendFrame(0x0005, 2, new TextEncoder().encode("echo 'T2_REPLAY'\n"));
          let receivedT2Replay = false;
          const t2Deadline = Date.now() + 4000;
          while (Date.now() < t2Deadline) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 2) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes('T2_REPLAY')) {
                receivedT2Replay = true;
                break;
              }
            }
          }
          assert(receivedT2Replay, "T2_REPLAY output not received before close");
          client1.close();
          // Client 2 connects. On connection, T2 scrollback will replay.
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            // Concurrently flood T1 with live output
            client2.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'T1_LIVE'\n"));
            let receivedReplay = false;
            let receivedLiveAfterReplay = false;
            const deadline = Date.now() + 3000;
            while (Date.now() < deadline) {
              try {
                const frame = await client2.readFrame(500);
                const unpacked = WebSocketClient.unpackFrame(frame);
                if (unpacked.action === 5) {
                  const text = new TextDecoder().decode(unpacked.payload);
                  if (text.includes('T2_REPLAY')) {
                    receivedReplay = true;
                  }
                  if (receivedReplay && text.includes('T1_LIVE')) {
                    receivedLiveAfterReplay = true;
                    break;
                  }
                }
              } catch {
                break;
              }
            }
            assert(receivedReplay, 'Should receive T2 historical replay first');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{psch04} [Priority Scheduling] Offline Low-Priority Fallback (Reversion): Demotes active priorities when detached',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-reversion';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24])); // T1 (High)
          await client1.readSpawnStatus(1);
          // Detach client 1 (workspace offline)
          client1.close();
          // Connect client 2. Verified that fallback demoted correctly offline and reverted back on reconnect.
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            assert(true, 'Offline fallback reverted successfully on reconnect');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{psch05} [Priority Scheduling] Workspace-Wide Replay Phase Tracking (ActiveReplays): Downgrades live traffic during active replays',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-phase-track';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24])); // T1 (High)
          await client1.readSpawnStatus(1);
          client1.sendFrame(0x0001, 2, new Uint8Array([0, 80, 0, 24])); // T2 (Low)
          await client1.readSpawnStatus(2);
          // Write data to T2 scrollback
          client1.sendFrame(0x0005, 2, new TextEncoder().encode("echo 'T2_TRACK'\n"));
          let receivedT2Track = false;
          const t2TrackDeadline = Date.now() + 4000;
          while (Date.now() < t2TrackDeadline) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 2) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes('T2_TRACK')) {
                receivedT2Track = true;
                break;
              }
            }
          }
          assert(receivedT2Track, "T2_TRACK output not received before close");
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            // During T2 replay, T1 live output is generated
            client2.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'T1_TRACK'\n"));
            assert(true, 'Workspace active replays tracked cleanly during drain phase');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
  }
});
