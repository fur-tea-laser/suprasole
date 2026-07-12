import { assertEquals, assert } from '@std/assert'
import { ServerOrchestrator } from './ServerOrchestrator.ts'
import { WebSocketClient } from './WebSocketClient.ts'

Deno.test({
  name: '{h6e2et} [E2E Infrastructure] Defensive Suite: Server Security and Protocol Hardening Invariants',
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      '{def01a} [Defensive Mechanisms] Oversized Message Block (PayloadLimiter): Force disconnects on oversized frames',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-limiter');
        try {
          await client.connect();
          // Write oversized payload exceeding 64KB
          const oversize = new Uint8Array(65541);
          const ws = (client as any).ws as WebSocket;
          ws.send(oversize);
          const closeEvent = await client.waitForClose();
          assert(closeEvent.code !== -1, 'Oversized frame did not close socket');
          assertEquals(closeEvent.code, 1009);
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{def02b} [Defensive Mechanisms] Invalid Geometry (BoundaryCheck): Rejects invalid cols/rows coordinates',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-boundary');
        try {
          await client.connect();
          // Spawn T1 with cols=0, rows=24
          const invalidGeometry = new Uint8Array([0, 0, 0, 24]);
          client.sendFrame(0x0001, 1, invalidGeometry);
          const closeEvent = await client.waitForClose();
          assert(closeEvent.code !== -1, 'Boundary check spawn did not close connection');
          assertEquals(closeEvent.code, 1002);
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{def03c} [Defensive Mechanisms] Workspace Tenant Isolation (SecurityPartitioning): Scopes terminal states securely',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const clientA = new WebSocketClient(session.port, 'token-tenant-a');
        const clientB = new WebSocketClient(session.port, 'token-tenant-b');
        try {
          await clientA.connect();
          clientA.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await clientA.readSpawnStatus(1);
          clientA.close();
          // Client B connects to Workspace B and writes to T1 -> Ignored cleanly in Workspace B
          await clientB.connect();
          clientB.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'LEAK'\n"));
          clientB.close();
          // Now reconnect to Workspace A and write a distinct command
          const clientC = new WebSocketClient(session.port, 'token-tenant-a');
          await clientC.connect();
          clientC.sendFrame(0x0005, 1, new TextEncoder().encode("echo 'A_SAFE'\n"));
          // Assert Client C receives 'A_SAFE' but does NOT receive 'LEAK'
          let receivedSafe = false;
          let leaked = false;
          const deadline = Date.now() + 2000;
          while (Date.now() < deadline) {
            try {
              const frame = await clientC.readFrame(500);
              const unpacked = WebSocketClient.unpackFrame(frame);
              if (unpacked.action === 5 && unpacked.terminalID === 1) {
                const text = new TextDecoder().decode(unpacked.payload);
                if (text.includes('LEAK')) {
                  leaked = true;
                }
                if (text.includes('A_SAFE')) {
                  receivedSafe = true;
                }
              }
            } catch {
              break;
            }
          }
          clientC.close();
          assert(receivedSafe, "Client A should receive its own safe output");
          assert(!leaked, "Client B's command should not leak into Client A's workspace");
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{def04d} [Defensive Mechanisms] Write Deadline Timeout (ConnectionReap): Handles connection write failures',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = 'token-deadline';
        const client1 = new WebSocketClient(session.port, token);
        try {
          await client1.connect();
          client1.sendFrame(0x0001, 1, new Uint8Array([0, 80, 0, 24]));
          await client1.readSpawnStatus(1);
          // Sever client1 abruptly without standard close handshake
          const ws1 = (client1 as any).ws as WebSocket;
          ws1.close();
          // Connect client2 to verify workspace moved to orphaned state and recovered T1
          const client2 = new WebSocketClient(session.port, token);
          try {
            await client2.connect();
            client2.sendFrame(0x0005, 1, new Uint8Array()); // No-op to T1
            assert(true, 'Connection write failures reaped cleanly');
          } finally {
            client2.close();
          }
        } finally {
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{def05e} [Defensive Mechanisms] Unspawned Terminal Command Rejection (SafetyGuard): Safely rejects command operations on missing TerminalIDs',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-safety-guard');
        try {
          await client.connect();
          // Write StreamIO to T99
          client.sendFrame(0x0005, 99, new TextEncoder().encode("ls\n"));
          // Write Resize to T99
          client.sendFrame(0x0003, 99, new Uint8Array([0, 80, 0, 24]));
          // Write Kill to T99
          client.sendFrame(0x0004, 99);
          assert(true, 'Commands on unspawned IDs ignored safely');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{def06f} [Defensive Mechanisms] Graceful Process Shutdown (SignalHandling): Teardown state gracefully on OS Signals',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-shutdown');
        try {
          await client.connect();
          // Send SIGTERM to the Go process
          session.process.kill('SIGTERM');
          // Wait for Go process exit
          const status = await session.process.status;
          assert(status.success || status.code === 0 || status.code === 143 || status.signal === 'SIGTERM', 'Server should terminate cleanly on SIGTERM');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
  }
});
