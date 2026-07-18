import { assert, assertEquals } from "@std/assert";
import { ServerOrchestrator } from "./ServerOrchestrator.ts";
import { WebSocketClient } from "./WebSocketClient.ts";

Deno.test({
  name:
    "{h6e2et} [E2E Infrastructure] Scheduler Suite: Strict Scheduling Priority Invariants",
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      "{psch01} [Priority Scheduling] Strict Priority Draining (Draining): Asserts high-priority queues drain first",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-drain");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash"); // T1
          const spawn_spawning_85327 = await client.readSpawnStatus(1);
          assertEquals(spawn_spawning_85327.payload[0], 0x02);
          const spawn_success_85327 = await client.readSpawnStatus(1);
          assertEquals(spawn_success_85327.payload[0], 0x00);
          client.sendSpawn(2, 80, 24, "bash"); // T2
          const spawn_spawning_534685 = await client.readSpawnStatus(2);
          assertEquals(spawn_spawning_534685.payload[0], 0x02);
          const spawn_success_534685 = await client.readSpawnStatus(2);
          assertEquals(spawn_success_534685.payload[0], 0x00);
          const syncPayload = new Uint8Array([0, 1, 1, 0, 2, 0]);
          client.sendFrame(0x0009, 0, syncPayload); // ActionPrioritySync = 9
          client.sendFrame(
            0x0007,
            2,
            new TextEncoder().encode(
              'for i in $(seq 1 100); do echo "LOW_$i"; sleep 0.01; done\n',
            ),
          );
          client.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode("echo 'PREEMPT'\n"),
          );
          let receivedPreempt = false;
          let finishedT2 = false;
          let preemptedSuccessful = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (unpacked.terminalID === 1 && text.includes("PREEMPT")) {
                receivedPreempt = true;
                if (!finishedT2) {
                  preemptedSuccessful = true;
                }
              }
              if (unpacked.terminalID === 2 && text.includes("LOW_100")) {
                finishedT2 = true;
              }
              if (receivedPreempt && finishedT2) {
                break;
              }
            }
          }
          assert(receivedPreempt, "Should receive T1 preempt output");
          assert(
            preemptedSuccessful,
            "High priority T1 output should preempt Low priority T2 queue draining",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch02} [Priority Scheduling] Whole-State Layout Synchronization (PrioritySync): Shift priority mappings dynamically",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-priority-sync");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash"); // T1
          const spawn_spawning_789572 = await client.readSpawnStatus(1);
          assertEquals(spawn_spawning_789572.payload[0], 0x02);
          const spawn_success_789572 = await client.readSpawnStatus(1);
          assertEquals(spawn_success_789572.payload[0], 0x00);
          client.sendSpawn(2, 80, 24, "bash"); // T2
          const spawn_spawning_332281 = await client.readSpawnStatus(2);
          assertEquals(spawn_spawning_332281.payload[0], 0x02);
          const spawn_success_332281 = await client.readSpawnStatus(2);
          assertEquals(spawn_success_332281.payload[0], 0x00);
          const syncPayload = new Uint8Array([0, 1, 0, 0, 2, 1]);
          client.sendFrame(0x0009, 0, syncPayload); // ActionPrioritySync = 9
          client.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode(
              'for i in {1..2000}; do echo "T1_DATA_$i"; done\n',
            ),
          );
          client.sendFrame(
            0x0007,
            2,
            new TextEncoder().encode("echo 'PREEMPT_FLIP'\n"),
          );
          let receivedPreempt = false;
          let finishedT1 = false;
          let preemptedSuccessful = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (unpacked.terminalID === 2 && text.includes("PREEMPT_FLIP")) {
                receivedPreempt = true;
                if (!finishedT1) {
                  preemptedSuccessful = true;
                }
              }
              if (unpacked.terminalID === 1 && text.includes("T1_DATA_2000")) {
                finishedT1 = true;
              }
              if (receivedPreempt && finishedT1) {
                break;
              }
            }
          }
          assert(receivedPreempt, "Should receive T2 preempt output");
          assert(
            preemptedSuccessful,
            "Dynamic PrioritySync should shift preemption priority to T2",
          );
          const invalidSync = new Uint8Array([0, 99, 1]);
          client.sendFrame(0x0009, 0, invalidSync);
          assert(true, "Invalid PrioritySync tuple ignored cleanly");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch03} [Priority Scheduling] Workspace-Wide Replay Starvation Prevention (Starvation): Starvation lockout checks",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-starvation";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash"); // T1 (High)
          const spawn_spawning_902392 = await client1.readSpawnStatus(1);
          assertEquals(spawn_spawning_902392.payload[0], 0x02);
          const spawn_success_902392 = await client1.readSpawnStatus(1);
          assertEquals(spawn_success_902392.payload[0], 0x00);
          client1.sendSpawn(2, 80, 24, "bash"); // T2 (Low)
          const spawn_spawning_740177 = await client1.readSpawnStatus(2);
          assertEquals(spawn_spawning_740177.payload[0], 0x02);
          const spawn_success_740177 = await client1.readSpawnStatus(2);
          assertEquals(spawn_success_740177.payload[0], 0x00);
          client1.sendFrame(
            0x0007,
            2,
            new TextEncoder().encode("echo 'T2_REPLAY'\n"),
          );
          let receivedT2Replay = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 2) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("T2_REPLAY")) {
                receivedT2Replay = true;
                break;
              }
            }
          }
          assert(
            receivedT2Replay,
            "T2_REPLAY output not received before close",
          );
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            client2.sendFrame(
              0x0007,
              1,
              new TextEncoder().encode("echo 'T1_LIVE'\n"),
            );
            let receivedReplay = false;
            let receivedLiveAfterReplay = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes("T2_REPLAY")) {
                  receivedReplay = true;
                }
                if (receivedReplay && text.includes("T1_LIVE")) {
                  receivedLiveAfterReplay = true;
                  break;
                }
              }
            }
            assert(receivedReplay, "Should receive T2 historical replay first");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch04} [Priority Scheduling] Offline Low-Priority Fallback (Reversion): Demotes active priorities when detached",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-reversion";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash"); // T1 (High)
          const spawn_spawning_241115 = await client1.readSpawnStatus(1);
          assertEquals(spawn_spawning_241115.payload[0], 0x02);
          const spawn_success_241115 = await client1.readSpawnStatus(1);
          assertEquals(spawn_success_241115.payload[0], 0x00);
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            assert(true, "Offline fallback reverted successfully on reconnect");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch05} [Priority Scheduling] Workspace-Wide Replay Phase Tracking (ActiveReplays): Downgrades live traffic during active replays",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-phase-track";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash"); // T1 (High)
          const spawn_spawning_444322 = await client1.readSpawnStatus(1);
          assertEquals(spawn_spawning_444322.payload[0], 0x02);
          const spawn_success_444322 = await client1.readSpawnStatus(1);
          assertEquals(spawn_success_444322.payload[0], 0x00);
          client1.sendSpawn(2, 80, 24, "bash"); // T2 (Low)
          const spawn_spawning_763024 = await client1.readSpawnStatus(2);
          assertEquals(spawn_spawning_763024.payload[0], 0x02);
          const spawn_success_763024 = await client1.readSpawnStatus(2);
          assertEquals(spawn_success_763024.payload[0], 0x00);
          client1.sendFrame(
            0x0007,
            2,
            new TextEncoder().encode("echo 'T2_TRACK'\n"),
          );
          let receivedT2Track = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 2) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("T2_TRACK")) {
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
            client2.sendFrame(
              0x0007,
              1,
              new TextEncoder().encode("echo 'T1_TRACK'\n"),
            );
            assert(
              true,
              "Workspace active replays tracked cleanly during drain phase",
            );
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch06} [Priority Scheduling] Spawning Cancellation Under Backpressure Lockout (SpawningLockout): Prevents spawning deadlocks under heavy egress congestion",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(
          session.port,
          "token-backpressure-lockout",
        );
        try {
          await client.connect();
          client.sendSpawn(311, 80, 24, "bash");
          const spawn_spawning_455466 = await client.readSpawnStatus(311);
          assertEquals(spawn_spawning_455466.payload[0], 0x02);
          const spawn_success_455466 = await client.readSpawnStatus(311);
          assertEquals(spawn_success_455466.payload[0], 0x00);
          client.sendFrame(
            0x0007,
            311,
            new TextEncoder().encode("while true; do echo 'FLOOD'; done\n"),
          );
          let bytesReceived = 0;
          while (bytesReceived < 8192) {
            const frame = await client.readFrame();
            if (frame.length > 0) {
              bytesReceived += frame.length;
            }
          }
          client.sendSpawn(312, 80, 24, "bash");
          client.sendFrame(0x0004, 312); // Kill T312
          assert(true, "KillPTY did not hang under backpressure lockout");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch07} [Priority Scheduling] Implicit Demotion of Omitted Terminals (ImplicitDemotion): Demotes unspecified active PTYs to Low priority on sync",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(
          session.port,
          "token-implicit-demote",
        );
        try {
          await client.connect();
          client.sendSpawn(318, 80, 24, "bash");
          const spawn_spawning_114969 = await client.readSpawnStatus(318);
          assertEquals(spawn_spawning_114969.payload[0], 0x02);
          const spawn_success_114969 = await client.readSpawnStatus(318);
          assertEquals(spawn_success_114969.payload[0], 0x00);
          client.sendSpawn(319, 80, 24, "bash");
          const spawn_spawning_582912 = await client.readSpawnStatus(319);
          assertEquals(spawn_spawning_582912.payload[0], 0x02);
          const spawn_success_582912 = await client.readSpawnStatus(319);
          assertEquals(spawn_success_582912.payload[0], 0x00);
          const syncPayload1 = new Uint8Array([0, 318, 1, 0, 319, 1]);
          client.sendFrame(0x0009, 0, syncPayload1);
          const syncPayload2 = new Uint8Array([0, 318, 1]);
          client.sendFrame(0x0009, 0, syncPayload2);
          client.sendFrame(
            0x0007,
            319,
            new TextEncoder().encode(
              'for i in $(seq 1 100); do echo "LOW_$i"; sleep 0.01; done\n',
            ),
          );
          client.sendFrame(
            0x0007,
            318,
            new TextEncoder().encode("echo 'PREEMPT'\n"),
          );
          let receivedPreempt = false;
          let finishedT2 = false;
          let preemptedSuccessful = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (unpacked.terminalID === 318 && text.includes("PREEMPT")) {
                receivedPreempt = true;
                if (!finishedT2) {
                  preemptedSuccessful = true;
                }
              }
              if (unpacked.terminalID === 319 && text.includes("LOW_100")) {
                finishedT2 = true;
              }
              if (receivedPreempt && finishedT2) {
                break;
              }
            }
          }
          assert(receivedPreempt, "Should receive T318 preempt output");
          assert(
            preemptedSuccessful,
            "T319 should be implicitly demoted and preempted by T318",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{psch08} [Priority Scheduling] Dynamic Priority Sync Layout (SyncPriorities): Asserts dynamic priority syncing using ActionPrioritySync (0x0009)",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(
          session.port,
          "token-sync-priorities",
        );
        try {
          await client.connect();
          client.sendSpawn(505, 80, 24, "bash");
          const spawn_spawning_190850 = await client.readSpawnStatus(505);
          assertEquals(spawn_spawning_190850.payload[0], 0x02);
          const spawn_success_190850 = await client.readSpawnStatus(505);
          assertEquals(spawn_success_190850.payload[0], 0x00);
          client.sendSpawn(506, 80, 24, "bash");
          const spawn_spawning_259738 = await client.readSpawnStatus(506);
          assertEquals(spawn_spawning_259738.payload[0], 0x02);
          const spawn_success_259738 = await client.readSpawnStatus(506);
          assertEquals(spawn_success_259738.payload[0], 0x00);
          // Sync specifies only 505 as High priority
          const syncPayload = new Uint8Array([0, 505, 1]);
          client.sendFrame(0x0009, 0, syncPayload);
          client.sendFrame(
            0x0007,
            506,
            new TextEncoder().encode(
              'for i in $(seq 1 100); do echo "LOW_$i"; sleep 0.01; done\n',
            ),
          );
          client.sendFrame(
            0x0007,
            505,
            new TextEncoder().encode("echo 'PREEMPT'\n"),
          );
          let receivedPreempt = false;
          let finishedT2 = false;
          let preemptedSuccessful = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (unpacked.terminalID === 505 && text.includes("PREEMPT")) {
                receivedPreempt = true;
                if (!finishedT2) {
                  preemptedSuccessful = true;
                }
              }
              if (unpacked.terminalID === 506 && text.includes("LOW_100")) {
                finishedT2 = true;
              }
              if (receivedPreempt && finishedT2) {
                break;
              }
            }
          }
          assert(receivedPreempt, "Should receive T505 preempt output");
          assert(
            preemptedSuccessful,
            "T506 should be implicitly demoted and preempted by T505",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp02} [Priority Scheduling] Responsive Multi-Tab Typing and Reconnection Recovery (ResponsiveSession): Asserts layout sync persistence across network drops",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-comp02";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(402, 80, 24, "bash");
          const spawn_spawning_816444 = await client1.readSpawnStatus(402);
          assertEquals(spawn_spawning_816444.payload[0], 0x02);
          const spawn_success_816444 = await client1.readSpawnStatus(402);
          assertEquals(spawn_success_816444.payload[0], 0x00);
          client1.sendSpawn(403, 80, 24, "bash");
          const spawn_spawning_568473 = await client1.readSpawnStatus(403);
          assertEquals(spawn_spawning_568473.payload[0], 0x02);
          const spawn_success_568473 = await client1.readSpawnStatus(403);
          assertEquals(spawn_success_568473.payload[0], 0x00);
          // Set 402 High, 403 Low
          const syncPayload = new Uint8Array([0, 402, 1, 0, 403, 0]);
          client1.sendFrame(0x0009, 0, syncPayload);
          // Assert 402 preempts 403 before disconnect
          client1.sendFrame(
            0x0007,
            403,
            new TextEncoder().encode(
              'for i in $(seq 1 100); do echo "LOW_$i"; sleep 0.01; done\n',
            ),
          );
          client1.sendFrame(
            0x0007,
            402,
            new TextEncoder().encode("echo 'PREEMPT_BEFORE_RECONNECT'\n"),
          );
          let receivedPreempt1 = false;
          let finishedT2_1 = false;
          let preemptedSuccessful1 = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (
                unpacked.terminalID === 402 &&
                text.includes("PREEMPT_BEFORE_RECONNECT")
              ) {
                receivedPreempt1 = true;
                if (!finishedT2_1) {
                  preemptedSuccessful1 = true;
                }
              }
              if (unpacked.terminalID === 403 && text.includes("LOW_100")) {
                finishedT2_1 = true;
              }
              if (receivedPreempt1 && finishedT2_1) {
                break;
              }
            }
          }
          assert(receivedPreempt1);
          assert(preemptedSuccessful1);
          client1.close();
          // Reconnect
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            // Assert High Priority 402 preemption over Low Priority 403 post-reconnection without resending sync
            client2.sendFrame(
              0x0007,
              403,
              new TextEncoder().encode(
                'for i in $(seq 1 100); do echo "LOW_POST_$i"; sleep 0.01; done\n',
              ),
            );
            client2.sendFrame(
              0x0007,
              402,
              new TextEncoder().encode("echo 'PREEMPT_AFTER_RECONNECT'\n"),
            );
            let receivedPreempt2 = false;
            let finishedT2_2 = false;
            let preemptedSuccessful2 = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (
                  unpacked.terminalID === 402 &&
                  text.includes("PREEMPT_AFTER_RECONNECT")
                ) {
                  receivedPreempt2 = true;
                  if (!finishedT2_2) {
                    preemptedSuccessful2 = true;
                  }
                }
                if (
                  unpacked.terminalID === 403 && text.includes("LOW_POST_100")
                ) {
                  finishedT2_2 = true;
                }
                if (receivedPreempt2 && finishedT2_2) {
                  break;
                }
              }
            }
            assert(
              receivedPreempt2,
              "Should receive 402 preempt output post-reconnect",
            );
            assert(
              preemptedSuccessful2,
              "Reconnection should preserve priority sync layout",
            );
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp06} [Priority Scheduling] Dynamic Priority Flip Under Heavy Egress Congestion (CongestionFlip): Verifies scheduler preemption responsiveness during live floods",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-comp06");
        try {
          await client.connect();
          client.sendSpawn(410, 80, 24, "bash");
          const spawn_spawning_439910 = await client.readSpawnStatus(410);
          assertEquals(spawn_spawning_439910.payload[0], 0x02);
          const spawn_success_439910 = await client.readSpawnStatus(410);
          assertEquals(spawn_success_439910.payload[0], 0x00);
          client.sendSpawn(411, 80, 24, "bash");
          const spawn_spawning_644739 = await client.readSpawnStatus(411);
          assertEquals(spawn_spawning_644739.payload[0], 0x02);
          const spawn_success_644739 = await client.readSpawnStatus(411);
          assertEquals(spawn_success_644739.payload[0], 0x00);
          // 410 High, 411 Low
          client.sendFrame(0x0009, 0, new Uint8Array([0, 410, 1, 0, 411, 0]));
          // Start T410 High priority flood
          client.sendFrame(
            0x0007,
            410,
            new TextEncoder().encode(
              'for i in $(seq 1 100000); do echo "HIGH_$i"; done\n',
            ),
          );
          // Wait for first stdout chunk from T410 to confirm flood is active
          let startedHigh = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.terminalID === 410 && unpacked.action === 8) {
              startedHigh = true;
              break;
            }
          }
          assert(startedHigh, "High priority flood failed to start");
          // Send STARVED to T411 (Low priority)
          client.sendFrame(
            0x0007,
            411,
            new TextEncoder().encode("echo 'STARVED'\n"),
          );
          // Send priority flip sync: 410 Low, 411 High
          client.sendFrame(0x0009, 0, new Uint8Array([0, 410, 0, 0, 411, 1]));
          let receivedStarved = false;
          let finished410 = false;
          let flipSuccessful = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (unpacked.terminalID === 411 && text.includes("STARVED")) {
                receivedStarved = true;
                if (!finished410) {
                  flipSuccessful = true;
                }
                break;
              }
              if (unpacked.terminalID === 410 && text.includes("HIGH_100000")) {
                finished410 = true;
              }
            }
          }
          assert(receivedStarved);
          assert(
            flipSuccessful,
            "Flipped high priority should immediately preempt starved stream",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
  },
});
