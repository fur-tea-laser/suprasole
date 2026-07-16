import { assert, assertEquals } from "@std/assert";
import { ServerOrchestrator } from "./ServerOrchestrator.ts";
import { WebSocketClient } from "./WebSocketClient.ts";

Deno.test({
  name:
    "{h6e2et} [E2E Infrastructure] Recovery Suite: Session Takeovers and Replays Invariants",
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      "{srec01} [Session Recovery] Reconnection Handshake & Takeover (Takeover): Forcefully evicts old session with 4000 close code",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-takeover-evict";
        const client1 = new WebSocketClient(session.port, token);
        const client2 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          await client2.connect();
          const closeEvent = await client1.waitForClose();
          assert(closeEvent.code !== -1, "Client 1 should be evicted");
          assertEquals(closeEvent.code, 4000);
          assertEquals(closeEvent.reason, "Session Taken Over");
        } finally {
          client1.close();
          client2.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{srec02} [Session Recovery] Historical Output Playback (Replay): Recovers cached scrollbacks",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-replay-history";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash");
          await client1.readSpawnStatus(1);
          client1.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode("echo 'REPLAY_TEST'\n"),
          );
          let receivedReplay = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("REPLAY_TEST")) {
                receivedReplay = true;
                break;
              }
            }
          }
          assert(
            receivedReplay,
            "REPLAY_TEST output not received before close",
          );
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let recovered = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8 && unpacked.terminalID === 1) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes("REPLAY_TEST")) {
                  recovered = true;
                  break;
                }
              }
            }
            assert(recovered, "Failed to recover historical logs");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{srec04} [Session Recovery] Scrollback Buffer Overflow Warning (Truncation): ANSI warning injection",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-overflow-legacy";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash");
          await client1.readSpawnStatus(1);
          // Write large stream to exceed 256KB ring buffer limit
          client1.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode("dd if=/dev/zero bs=300000 count=1\n"),
          );
          let totalBytes = 0;
          while (totalBytes < 256 * 1024) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              totalBytes += unpacked.payload.length;
            }
          }
          assert(totalBytes >= 256 * 1024);
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let truncated = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8 && unpacked.terminalID === 1) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (
                  text.includes(
                    "[... Output truncated due to buffer overflow ...]",
                  )
                ) {
                  truncated = true;
                  break;
                }
              }
            }
            assert(truncated, "Truncation warning was not injected");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{srec07} [Session Recovery] Reconnect Terminated Replay (ExitReplay): Replays exit status codes for terminated PTYs on connection takeover",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-exit-replay";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(307, 80, 24, "bash");
          await client1.readSpawnStatus(307);
          client1.sendFrame(0x0007, 307, new TextEncoder().encode("exit 42\n"));
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 307) {
              assertEquals(unpacked.payload[0], 42);
              break;
            }
          }
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let replayedExit = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 307) {
                assertEquals(unpacked.payload[0], 42);
                replayedExit = true;
                break;
              }
            }
            assert(
              replayedExit,
              "Exit status code was not replayed on reconnection",
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
      "{srec08} [Session Recovery] UTF-8 Scrollback Alignment (Sanitization): Sanitizes mid-character truncations on replay",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-utf8-sanitization";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(308, 80, 24, "cat");
          await client1.readSpawnStatus(308);
          const rawBytes = new Uint8Array([0x82, 0xBF, 0xE2, 0x82, 0xAC]);
          client1.sendFrame(0x0007, 308, rawBytes);
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.terminalID === 308 && unpacked.action === 8) {
              break;
            }
          }
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            const frame = await client2.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            assertEquals(unpacked.terminalID, 308);
            assertEquals(unpacked.action, 8);
            const sanitizedOutput = unpacked.payload;
            assertEquals(
              sanitizedOutput,
              new Uint8Array([0xE2, 0x82, 0xAC]),
              "Leading continuation bytes were not sanitized",
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
      "{srec09} [Session Recovery] Offline Exit Handler Unblocking (OfflineUnblock): Ensures immediate process reaping on disconnected sessions",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-offline-unblock";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(310, 80, 24, "bash");
          await client1.readSpawnStatus(310);
          client1.sendFrame(
            0x0007,
            310,
            new TextEncoder().encode("echo 'OFFLINE_TEST'; exit 0\n"),
          );
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.terminalID === 310 && unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("OFFLINE_TEST")) {
                break;
              }
            }
          }
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let gotExit = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.terminalID === 310 && unpacked.action === 5) {
                gotExit = true;
                break;
              }
            }
            assert(
              gotExit,
              "Terminal 310 was not cleanly reaped to Terminated state",
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
      "{srec10} [Session Recovery] Replay Starvation Prevention Workspace-Wide (ReplayStarvation): Places live traffic behind historical playback on reconnection",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-starvation-workspace";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(313, 80, 24, "bash");
          await client1.readSpawnStatus(313);
          client1.sendSpawn(314, 80, 24, "bash");
          await client1.readSpawnStatus(314);
          client1.sendFrame(
            0x0007,
            313,
            new TextEncoder().encode("echo 'T313_REPLAY'\n"),
          );
          let gotReplayText = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 313) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("T313_REPLAY")) {
                gotReplayText = true;
                break;
              }
            }
          }
          assert(gotReplayText);
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            client2.sendFrame(
              0x0007,
              314,
              new TextEncoder().encode("echo 'T314_LIVE'\n"),
            );
            let gotReplay = false;
            let gotLiveAfterReplay = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (
                  unpacked.terminalID === 313 && text.includes("T313_REPLAY")
                ) {
                  gotReplay = true;
                }
                if (
                  gotReplay && unpacked.terminalID === 314 &&
                  text.includes("T314_LIVE")
                ) {
                  gotLiveAfterReplay = true;
                  break;
                }
              }
            }
            assert(gotReplay, "Should receive terminal 313 replay");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp03} [Session Recovery] Reconnect Takeover Handshake Interleaving (InterleavedHandshake): Asserts correct delivery order of buffer warnings, exits, and async spawns",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-comp03";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(404, 80, 24, "bash");
          await client1.readSpawnStatus(404);
          client1.sendSpawn(405, 80, 24, "bash");
          await client1.readSpawnStatus(405);
          client1.sendFrame(
            0x0007,
            404,
            new TextEncoder().encode("dd if=/dev/zero bs=300000 count=1\n"),
          );
          let totalBytes = 0;
          while (totalBytes < 256 * 1024) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 404) {
              totalBytes += unpacked.payload.length;
            }
          }
          assert(totalBytes >= 256 * 1024);
          client1.sendFrame(0x0007, 405, new TextEncoder().encode("exit 77\n"));
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 405) {
              assertEquals(unpacked.payload[0], 77);
              break;
            }
          }
          client1.sendSpawn(406, 80, 24, "sleep", ["99999"]);
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            const events: {
              terminalID: number;
              action: number;
              text?: string;
              exitCode?: number;
            }[] = [];
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              const event: any = {
                terminalID: unpacked.terminalID,
                action: unpacked.action,
              };
              if (unpacked.action === 8) {
                event.text = new TextDecoder().decode(unpacked.payload);
              } else if (unpacked.action === 5) {
                event.exitCode = unpacked.payload[0];
              }
              events.push(event);
              const got404 = events.some((e) =>
                e.terminalID === 404 && e.action === 8 &&
                e.text?.includes(
                  "[... Output truncated due to buffer overflow ...]",
                )
              );
              const got405 = events.some((e) =>
                e.terminalID === 405 && e.action === 5 && e.exitCode === 77
              );
              const got406 = events.some((e) =>
                e.terminalID === 406 && e.action === 2
              );
              if (got404 && got405 && got406) {
                break;
              }
            }
            const idx404 = events.findIndex((e) =>
              e.terminalID === 404 && e.action === 8 &&
              e.text?.includes(
                "[... Output truncated due to buffer overflow ...]",
              )
            );
            const idx405 = events.findIndex((e) =>
              e.terminalID === 405 && e.action === 5 && e.exitCode === 77
            );
            const idx406 = events.findIndex((e) =>
              e.terminalID === 406 && e.action === 2
            );
            assert(idx404 !== -1, "Should receive 404 truncated warning");
            assert(idx405 !== -1, "Should receive 405 exit code 77");
            assert(idx406 !== -1, "Should receive 406 spawn success status");
            assert(
              idx406 > idx404,
              "406 spawn status must be received after 404 replay warning",
            );
            assert(
              idx406 > idx405,
              "406 spawn status must be received after 405 exit",
            );
          } finally {
            client1.close();
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp05} [Session Recovery] Takeover During Spawning With Immediate Eviction (TakeoverSpawnEvict): Asserts spawn abort and memory cleanup on sequential takeover events",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-comp05";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(409, 80, 24, "sleep", ["99999"]);
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            client2.sendFrame(0x0006, 409);
            client2.sendSpawn(409, 80, 24, "bash");
            const unpacked = await client2.readSpawnStatus(409);
            assertEquals(unpacked.payload[0], 0x00);
          } finally {
            client2.close();
          }
        } finally {
          client1.close();
          await session.shutdown();
        }
      },
    );
  },
});
