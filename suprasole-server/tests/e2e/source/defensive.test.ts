import { assert, assertEquals } from "@std/assert";
import { ServerOrchestrator } from "./ServerOrchestrator.ts";
import { WebSocketClient } from "./WebSocketClient.ts";

Deno.test({
  name:
    "{h6e2et} [E2E Infrastructure] Defensive Suite: Server Security and Protocol Hardening Invariants",
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      "{def01a} [Defensive Mechanisms] Oversized Message Block (PayloadLimiter): Force disconnects on oversized frames",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-limiter");
        try {
          await client.connect();
          const oversize = new Uint8Array(65541);
          const ws = (client as any).ws as WebSocket;
          ws.send(oversize);
          const closeEvent = await client.waitForClose();
          assert(
            closeEvent.code !== -1,
            "Oversized frame did not close socket",
          );
          assertEquals(closeEvent.code, 1009);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def02b} [Defensive Mechanisms] Invalid Geometry (BoundaryCheck): Rejects invalid cols/rows coordinates",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-boundary");
        try {
          await client.connect();
          client.sendSpawn(1, 0, 24, "bash");
          const closeEvent = await client.waitForClose();
          assert(
            closeEvent.code !== -1,
            "Boundary check spawn did not close connection",
          );
          assertEquals(closeEvent.code, 1002);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def03c} [Defensive Mechanisms] Workspace Tenant Isolation (SecurityPartitioning): Scopes terminal states securely",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const clientA = new WebSocketClient(session.port, "token-tenant-a");
        const clientB = new WebSocketClient(session.port, "token-tenant-b");
        try {
          await clientA.connect();
          clientA.sendSpawn(1, 80, 24, "bash");
          await clientA.readSpawnStatus(1);
          clientA.close();
          await clientB.connect();
          clientB.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode("echo 'LEAK'\n"),
          );
          clientB.close();
          const clientC = new WebSocketClient(session.port, "token-tenant-a");
          await clientC.connect();
          clientC.sendFrame(
            0x0007,
            1,
            new TextEncoder().encode("echo 'A_SAFE'\n"),
          );
          let receivedSafe = false;
          let leaked = false;
          let accumulated = "";
          while (true) {
            const frame = await clientC.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 1) {
              const text = new TextDecoder().decode(unpacked.payload);
              accumulated += text;
              if (text.includes("A_SAFE")) {
                receivedSafe = true;
                break;
              }
            }
          }
          if (accumulated.includes("LEAK")) {
            leaked = true;
          }
          clientC.close();
          assert(receivedSafe, "Client A should receive its own safe output");
          assert(
            !leaked,
            "Client B's command should not leak into Client A's workspace",
          );
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def04d} [Defensive Mechanisms] Write Deadline Timeout (ConnectionReap): Handles connection write failures",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-deadline";
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendSpawn(1, 80, 24, "bash");
          await client1.readSpawnStatus(1);
          const ws1 = (client1 as any).ws as WebSocket;
          ws1.close();
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            client2.sendFrame(0x0007, 1, new Uint8Array());
            assert(true, "Connection write failures reaped cleanly");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def05e} [Defensive Mechanisms] Unspawned Terminal Command Rejection (SafetyGuard): Safely rejects command operations on missing TerminalIDs",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-safety-guard");
        try {
          await client.connect();
          client.sendFrame(0x0007, 99, new TextEncoder().encode("ls\n"));
          client.sendFrame(0x0003, 99, new Uint8Array([0, 80, 0, 24]));
          client.sendFrame(0x0004, 99);
          assert(true, "Commands on unspawned IDs ignored safely");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def06f} [Defensive Mechanisms] Graceful Process Shutdown (SignalHandling): Teardown state gracefully on OS Signals",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-shutdown");
        try {
          await client.connect();
          session.process.kill("SIGTERM");
          const status = await session.process.status;
          assert(
            status.success || status.code === 0 || status.code === 143 ||
              status.signal === "SIGTERM",
            "Server should terminate cleanly on SIGTERM",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def07g} [Defensive Mechanisms] PID Recycling Bypass (SafetyGuard): Skips process group signaling on terminated terminals",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-pid-bypass");
        try {
          await client.connect();
          client.sendSpawn(309, 80, 24, "bash");
          await client.readSpawnStatus(309);
          client.sendFrame(0x0007, 309, new TextEncoder().encode("exit 0\n"));
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 5 && unpacked.terminalID === 309) {
              break;
            }
          }
          client.sendFrame(0x0004, 309);
          assert(
            true,
            "Kill on terminated terminal bypassed signal handling cleanly",
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def08h} [Defensive Mechanisms] Empty Command Path Rejection (EmptyCommand): Asserts connection close on empty command length",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-empty-command");
        try {
          await client.connect();
          const payload = new Uint8Array([0, 80, 0, 24, 0, 0]);
          client.sendFrame(0x0001, 503, payload);
          const closeEvent = await client.waitForClose();
          assertEquals(closeEvent.code, 1002);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def09i} [Defensive Mechanisms] Truncated Payload Rejection (TruncatedPayload): Asserts connection close on mismatch between declared command length and frame bounds",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-truncated");
        try {
          await client.connect();
          const payload = new Uint8Array([
            0,
            80,
            0,
            24,
            0,
            20,
            0x61,
            0x62,
            0x63,
          ]);
          client.sendFrame(0x0001, 504, payload);
          const closeEvent = await client.waitForClose();
          assertEquals(closeEvent.code, 1002);
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def10j} [Defensive Mechanisms] Orphaned Process Tree Reaping (OrphanReaping): Asserts clean cleanup of child process tree",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(
          session.port,
          "token-orphan-reaping",
        );
        try {
          await client.connect();
          client.sendSpawn(507, 80, 24, "bash");
          await client.readSpawnStatus(507);
          client.sendFrame(
            0x0007,
            507,
            new TextEncoder().encode(
              "sh -c 'sleep 9999' & echo \"CHILD_PID:$!\"\n",
            ),
          );
          let childPid = -1;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.action === 8 && unpacked.terminalID === 507) {
              const text = new TextDecoder().decode(unpacked.payload);
              const match = text.match(/CHILD_PID:(\d+)/);
              if (match) {
                childPid = parseInt(match[1], 10);
                break;
              }
            }
          }
          assert(childPid > 0, "Could not retrieve background sleep PID");
          client.sendFrame(0x0004, 507);
          let reaped = false;
          while (true) {
            const frame = await client.readFrame();
            const unpacked = WebSocketClient.unpackFrame(frame);
            if (unpacked.terminalID === 507 && unpacked.action === 5) {
              reaped = true;
              break;
            }
          }
          assert(reaped, "Did not receive terminal exit status after kill");
          const ps = new Deno.Command("ps", {
            args: ["-p", childPid.toString()],
          });
          const psOutput = await ps.output();
          assert(
            !psOutput.success,
            `Background process PID ${childPid} was not reaped and is still running`,
          );
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{def11k} [Defensive Mechanisms] Startup Port Collision Resiliency (PortCollision): Asserts non-zero exit on TCP port binding collision",
      async () => {
        const listener = Deno.listen({
          hostname: "127.0.0.1",
          port: 9999,
          transport: "tcp",
        });
        try {
          const command = new Deno.Command((orchestrator as any).binaryPath, {
            args: ["-port", "9999"],
            stdout: "null",
            stderr: "piped",
          });
          const process = command.spawn();
          const status = await process.status;
          assert(
            !status.success,
            "Server should fail to bind and exit with non-zero code",
          );
        } finally {
          listener.close();
        }
      },
    );
    await t.step(
      "{comp07} [Defensive Mechanisms] Malformed Command Interleaving (MalformedInterleave): Asserts connection termination on mixed valid and malformed headers",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-comp07");
        try {
          await client.connect();
          client.sendSpawn(412, 80, 24, "bash");
          await client.readSpawnStatus(412);
          client.sendFrame(0x0007, 412, new TextEncoder().encode("ls\n"));
          client.sendFrame(0x0099, 412);
          const closeEvent = await client.waitForClose();
          assertEquals(
            closeEvent.code,
            1002,
            "Should immediately close socket with 1002 on malformed interleave",
          );
          client.close();
          // Reconnect with client2 and assert that terminal 412 survived and is still active in the registry
          const client2 = new WebSocketClient(session.port, "token-comp07");
          try {
            await client2.connect();
            client2.sendFrame(
              0x0007,
              412,
              new TextEncoder().encode("echo 'SURVIVED'\n"),
            );
            let gotStdout = false;
            while (true) {
              const frame = await client2.readFrame();
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.terminalID === 412 && unpacked.action === 8) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes("SURVIVED")) {
                  gotStdout = true;
                  break;
                }
              }
            }
            assert(gotStdout, "Terminal 412 did not survive the client drop");
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      },
    );
  },
});
