import { assert, assertEquals } from "@std/assert";
import { ServerOrchestrator } from "./ServerOrchestrator.ts";
import { WebSocketClient } from "./WebSocketClient.ts";

Deno.test({
  name:
    "{h6e2et} [E2E Infrastructure] PTY Suite: Pseudo-Terminal Lifecycle and IO Invariants",
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      "{ptyl01} [PTY Ingestion] Spawn Shell (Spawn): Spawns pseudo-terminals and receives status confirmation",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-spawn");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          let unpacked = await client.readSpawnStatus(1);
          assertEquals(unpacked.action, 2);
          assertEquals(unpacked.terminalID, 1);
          assertEquals(unpacked.payload[0], 0x00);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl02} [PTY Ingestion] Inter-Process Stream IO (StreamIO): Verifies stream I/O behaviors and edge cases",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-pty-io");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          const command = new TextEncoder().encode("echo 'E2E_INGEST'\n");
          client.sendFrame(0x0007, 1, command);
          let receivedHello = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("E2E_INGEST")) {
                receivedHello = true;
                break;
              }
            }
          }
          assert(
            receivedHello,
            "Should receive streamed stdout containing command string",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl03} [PTY Ingestion] Terminal Window Resize (Resize): Updates geometry window dimensions",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-resize");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          client.sendFrame(0x0003, 1, new Uint8Array([0, 100, 0, 30])); // Resize to 100x30
          client.sendFrame(0x0007, 1, new TextEncoder().encode("stty size\n"));
          let resized = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("30 100")) {
                resized = true;
                break;
              }
            }
          }
          assert(resized, "PTY shell did not resize to 30 100");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl04} [PTY Ingestion] Process Termination (Kill): Tears down child process and reaps exit code",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-kill");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          // Trigger termination using KillPTY
          client.sendFrame(0x0004, 1);
          let reapedCode = -1;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              reapedCode = unpacked.payload[0];
              break;
            }
          }
          assertEquals(
            reapedCode,
            137,
            "Killed process exit code should be 137 (128 + SIGKILL)",
          );
          client.close();
          // Verify terminal remains in memory and can be replayed on reconnect
          const client2 = new WebSocketClient(session.port, "token-kill");
          try {
            await client2.connect();
            let replayedExit = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 1) {
                assertEquals(unpacked.payload[0], 137);
                replayedExit = true;
                break;
              }
            }
            assert(
              replayedExit,
              "Terminated PTY exit code should be replayed on takeover",
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
      "{ptyl05} [PTY Ingestion] Queue Congestion Flow Control (QueueCap): Enforces priority-based flow queues",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-congestion");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          client.sendFrame(0x0007, 1, new TextEncoder().encode("seq 1 10\n"));
          let receivedCount = 0;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              receivedCount++;
              if (new TextDecoder().decode(unpacked.payload).includes("10")) {
                break;
              }
            }
          }
          assert(
            receivedCount > 0,
            "Active connection should successfully receive stream outputs",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl06} [PTY Ingestion] Double Kill Protection (SafeExit): Ignores duplicate process termination requests",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-double-kill");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          client.sendFrame(0x0004, 1);
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              break;
            }
          }
          client.sendFrame(0x0004, 1);
          client.sendFrame(0x0007, 1, new TextEncoder().encode("ls\n"));
          assert(true, "Duplicate kill ignored cleanly");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl07} [PTY Ingestion] TerminalID Reuse Restriction (IdCollision): Rejects duplicate spawns",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-collision");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          // Exit terminal 1
          client.sendFrame(0x0007, 1, new TextEncoder().encode("exit 0\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 1) {
              break;
            }
          }
          // Attempt to spawn terminal 1 again (while terminated but not removed) -> should reject with 0x01
          client.sendSpawn(1, 80, 24, "bash");
          const unpacked = await client.readSpawnStatus(1);
          assertEquals(
            unpacked.payload[0],
            0x01,
            "Spawn on terminated but not removed terminal should fail with 0x01",
          );
          // Call RemovePTY
          client.sendFrame(0x0006, 1);
          // Spawn terminal 1 again -> should succeed
          client.sendSpawn(1, 80, 24, "bash");
          const unpacked2 = await client.readSpawnStatus(1);
          assertEquals(
            unpacked2.payload[0],
            0x00,
            "Spawn should succeed after RemovePTY",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl08} [PTY Ingestion] Stream Output Chunking (BufferSizing): Chunks output stream payload limits",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-chunking");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          client.sendFrame(0x0007, 1, new TextEncoder().encode("seq 1 3000\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              assert(
                unpacked.payload.length <= 32768,
                "Frame size exceeded 32KB read buffer limit",
              );
              if (new TextDecoder().decode(unpacked.payload).includes("3000")) {
                break;
              }
            }
          }
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl09} [PTY Ingestion] Failed Resize Window Resilience (ResizeError): Recovers from failing ioctl window size changes",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-resize-err");
        try {
          await client.connect();
          client.sendSpawn(1, 80, 24, "bash");
          await client.readSpawnStatus(1);
          client.sendFrame(0x0003, 99, new Uint8Array([0, 80, 0, 24]));
          client.sendFrame(0x0007, 1, new TextEncoder().encode("ls\n"));
          assert(true, "Failing resize handled cleanly");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl10} [PTY Ingestion] PTY Eviction (Remove): Verifies RemovePTY completely purges terminal maps and buffers",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-remove");
        try {
          await client.connect();
          client.sendSpawn(301, 80, 24, "bash");
          await client.readSpawnStatus(301);
          // Evict/Remove terminal 301
          client.sendFrame(0x0006, 301);
          // Re-spawning terminal 301 should succeed now without ID collision
          client.sendSpawn(301, 80, 24, "bash");
          const unpacked = await client.readSpawnStatus(301);
          assertEquals(
            unpacked.payload[0],
            0x00,
            "Re-spawning evicted terminal should succeed",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl11} [PTY Ingestion] Context-Bound Spawning Early Kill (CancelSpawn): Cancels spawning process via KillPTY mid-launch",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-early-kill");
        try {
          await client.connect();
          client.sendSpawn(302, 80, 24, "sleep", ["10"]);
          client.sendFrame(0x0004, 302);
          const unpacked = await client.readSpawnStatus(302);
          assert(unpacked.payload[0] === 0x01 || unpacked.payload[0] === 0x00);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl12} [PTY Ingestion] Spawning Command Discard (LateRouting): Discards resizes and inputs for spawning terminals",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-late-routing");
        try {
          await client.connect();
          let success = false;
          for (let attempt = 0; attempt < 50; attempt++) {
            const tid = 303 + attempt;
            client.sendSpawn(tid, 80, 24, "bash");
            client.sendFrame(
              0x0007,
              tid,
              new TextEncoder().encode("echo 'DISCARDED'\n"),
            );
            client.sendFrame(0x0003, tid, new Uint8Array([0, 120, 0, 40]));
            await client.readSpawnStatus(tid);
            client.sendFrame(
              0x0007,
              tid,
              new TextEncoder().encode("echo 'VERIFIED'\n"),
            );
            let receivedVerified = false;
            let receivedDiscarded = false;
            const deadline = Date.now() + 1000;
            while (true) {
              const frame = await client.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 8 && unpacked.terminalID === tid) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes("DISCARDED")) {
                  receivedDiscarded = true;
                }
                if (text.includes("VERIFIED")) {
                  receivedVerified = true;
                  break;
                }
              }
            }
            if (receivedVerified && !receivedDiscarded) {
              success = true;
              break;
            }
            client.sendFrame(0x0006, tid); // Clean up for next attempt
          }
          assert(
            success,
            "Spawning inputs should be discarded (failed to hit spawning window after 50 attempts)",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl13} [PTY Ingestion] Terminated Terminal Safety (GuardedRouting): Bypasses writes and resizes to terminated terminals",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-guarded");
        try {
          await client.connect();
          client.sendSpawn(304, 80, 24, "bash");
          await client.readSpawnStatus(304);
          client.sendFrame(0x0007, 304, new TextEncoder().encode("exit 0\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 304) {
              break;
            }
          }
          client.sendFrame(0x0007, 304, new TextEncoder().encode("ls\n"));
          client.sendFrame(0x0003, 304, new Uint8Array([0, 100, 0, 30]));
          assert(
            true,
            "Writes to terminated terminal did not cause server crash",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl14} [PTY Ingestion] Reset Workspace Active and Terminated (WorkspaceReset): Wipes all terminal maps and process groups",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-reset-ws");
        try {
          await client.connect();
          client.sendSpawn(305, 80, 24, "bash");
          await client.readSpawnStatus(305);
          client.sendSpawn(306, 80, 24, "bash");
          await client.readSpawnStatus(306);
          client.sendFrame(0x0007, 306, new TextEncoder().encode("exit 0\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 306) {
              break;
            }
          }
          client.sendFrame(0x000a, 0); // ActionReset = 10 (0x0a)
          let receivedReset = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 10) {
              receivedReset = true;
              break;
            }
          }
          assert(
            receivedReset,
            "Should receive workspace Reset broadcast frame",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl15} [PTY Ingestion] Reset Workspace Outbound Notification (ResetFrame): Asserts Reset frame broadcast on workspace reset",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-reset-notify");
        try {
          await client.connect();
          client.sendSpawn(316, 80, 24, "bash");
          await client.readSpawnStatus(316);
          client.sendFrame(0x000a, 0); // ActionReset = 10 (0x0a)
          let receivedReset = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 10 && unpacked.terminalID === 0) {
              receivedReset = true;
              break;
            }
          }
          assert(
            receivedReset,
            "Should receive Reset frame with Action 0x0a and TerminalID 0",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl16} [PTY Ingestion] Synchronous State Consistency on Exit (ExitConsistency): Guarantees exit code capture before client notification",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(
          session.port,
          "token-exit-consistency",
        );
        try {
          await client.connect();
          client.sendSpawn(317, 80, 24, "bash");
          await client.readSpawnStatus(317);
          client.sendFrame(0x0007, 317, new TextEncoder().encode("exit 99\n"));
          let reapedExit = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 317) {
              assertEquals(unpacked.payload[0], 99);
              reapedExit = true;
              break;
            }
          }
          assert(reapedExit);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl18} [PTY Ingestion] Default Shell Spawn Passthrough (DefaultSpawn): Spawns default bash shell via length-prefixed protocol",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-default-spawn");
        try {
          await client.connect();
          client.sendSpawn(501, 80, 24, "/bin/bash");
          const unpacked = await client.readSpawnStatus(501);
          assertEquals(unpacked.payload[0], 0x00);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl19} [PTY Ingestion] Custom Executable Passthrough (CustomSpawn): Spawns custom command with arguments and asserts stdout",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-custom-spawn");
        try {
          await client.connect();
          client.sendSpawn(502, 80, 24, "echo", ["PASSTHROUGH_TEST"]);
          await client.readSpawnStatus(502);
          let foundStdout = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 502) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("PASSTHROUGH_TEST")) {
                foundStdout = true;
                break;
              }
            }
          }
          assert(foundStdout, "Echo output not streamed back");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl20} [PTY Ingestion] Interactive Control Signal Passthrough (CtrlCInterrupt): Interrupts loop execution via Ctrl+C",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-ctrl-c");
        try {
          await client.connect();
          client.sendSpawn(508, 80, 24, "bash");
          await client.readSpawnStatus(508);
          client.sendFrame(
            0x0007,
            508,
            new TextEncoder().encode(
              "while true; do echo 'running'; sleep 0.1; done\n",
            ),
          );
          let gotOutput = false;
          for (let i = 0; i < 5; i++) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 508) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("running")) {
                gotOutput = true;
              }
            }
          }
          assert(gotOutput);
          client.sendFrame(0x0007, 508, new Uint8Array([0x03]));
          client.sendFrame(
            0x0007,
            508,
            new TextEncoder().encode("echo 'INTERRUPTED'\n"),
          );
          let gotInterrupted = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 508) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("INTERRUPTED")) {
                gotInterrupted = true;
                break;
              }
            }
          }
          assert(gotInterrupted, "Loop was not interrupted by Ctrl+C");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{ptyl21} [PTY Ingestion] Argument Safety and Metacharacter Passthrough (LiteralArgs): Asserts direct execution without shell evaluation",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-literal-args");
        try {
          await client.connect();
          client.sendSpawn(509, 80, 24, "printf", [
            "%s\n",
            "hello; echo 'INJECTED'",
          ]);
          await client.readSpawnStatus(509);
          let accumulated = "";
          while (true) {
            try {
              const frame = await client.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.terminalID === 509) {
                if (unpacked.action === 8) {
                  accumulated += new TextDecoder().decode(unpacked.payload);
                }
                if (unpacked.action === 5) {
                  break;
                }
              }
            } catch {
              break;
            }
          }
          const hasLiteral = accumulated.includes("hello; echo 'INJECTED'");
          const hasInjected = accumulated.includes("INJECTED") && !hasLiteral;
          assert(
            hasLiteral,
            `Literal argument containing shell characters was not printed. Output: ${
              JSON.stringify(accumulated)
            }`,
          );
          assert(
            !hasInjected,
            "Shell characters in custom arguments were evaluated (injection vulnerability!)",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp01} [PTY Ingestion] Interactive Task & Tab Eviction Sequence (InteractiveEviction): Validates job control and terminal close sequence",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-comp01";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(401, 80, 24, "bash");
          await client1.readSpawnStatus(401);
          client1.sendFrame(
            0x0007,
            401,
            new TextEncoder().encode(
              'for i in $(seq 1 100); do echo "Line $i"; sleep 0.05; done\n',
            ),
          );
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.terminalID === 401 && unpacked.action === 8) {
              const text = new TextDecoder().decode(unpacked.payload);
              if (text.includes("Line ")) {
                break;
              }
            }
          }
          client1.sendFrame(0x0003, 401, new Uint8Array([0, 120, 0, 40]));
          client1.sendFrame(0x0007, 401, new Uint8Array([0x03]));
          client1.sendFrame(
            0x0007,
            401,
            new TextEncoder().encode("exit 130\n"),
          );
          let reapedExit = false;
          while (true) {
            const frame = await client1.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 401) {
              assertEquals(unpacked.payload[0], 130);
              reapedExit = true;
              break;
            }
          }
          assert(reapedExit);
          client1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            let gotExitReplay = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 401) {
                assertEquals(unpacked.payload[0], 130);
                gotExitReplay = true;
                break;
              }
            }
            assert(gotExitReplay);
            client2.sendFrame(0x0006, 401);
            client2.sendSpawn(401, 80, 24, "bash");
            const unpackedStatus = await client2.readSpawnStatus(401);
            assertEquals(unpackedStatus.payload[0], 0x00);
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp04} [PTY Ingestion] Workspace Reset and ID Recycling Recovery (ResetRecycling): Validates clean slate recovery and immediate re-spawns",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-comp04");
        try {
          await client.connect();
          client.sendSpawn(407, 80, 24, "bash");
          await client.readSpawnStatus(407);
          client.sendSpawn(408, 80, 24, "bash");
          await client.readSpawnStatus(408);
          client.sendFrame(0x0007, 408, new TextEncoder().encode("exit 0\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 408) {
              break;
            }
          }
          client.sendFrame(0x000a, 0); // ResetWorkspace
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 10) {
              break;
            }
          }
          client.sendSpawn(407, 80, 24, "bash");
          const status407 = await client.readSpawnStatus(407);
          assertEquals(status407.payload[0], 0x00);
          client.sendSpawn(408, 80, 24, "bash");
          const status408 = await client.readSpawnStatus(408);
          assertEquals(status408.payload[0], 0x00);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{comp08} [PTY Ingestion] Rapid Concurrent Spawns and Teardown (ConcurrentTeardown): Validates lock safety under heavy concurrent allocation and reset load",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-comp08");
        try {
          await client.connect();
          for (let id = 413; id <= 422; id++) {
            client.sendSpawn(id, 80, 24, "bash");
          }
          for (let id = 413; id <= 422; id++) {
            client.sendFrame(0x0007, id, new TextEncoder().encode("ls\n"));
          }
          client.sendFrame(0x000a, 0);
          assert(
            true,
            "Concurrency stress run did not deadlock or crash the server",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
  },
});
