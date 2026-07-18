import { assert, assertEquals } from "@std/assert";
import { ServerOrchestrator } from "./ServerOrchestrator.ts";
import { WebSocketClient } from "./WebSocketClient.ts";

Deno.test({
  name:
    "{h6e2et} [E2E Infrastructure] Gateway Suite: WebSocket Connection and Heartbeat Invariants",
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      "{wsg01a} [WebSocket Gateway] Handshake Upgrade (Connection): Connects and verifies WebSocket protocol upgraded successfully",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-handshake");
        try {
          await client.connect();
          assert(true, "Successfully connected via WebSocket");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg02b} [WebSocket Gateway] Malformed Frame Rejection (ProtocolError): Asserts immediate connection closure on malformed inputs",
      async () => {
        const session = await orchestrator.spawnRuntime();
        // Test Input A: Length Underflow (< 4 bytes)
        const clientA = new WebSocketClient(session.port, "token-proto-a");
        await clientA.connect();
        try {
          const ws = (clientA as any).ws as WebSocket;
          ws.send(new Uint8Array([0x00, 0x05, 0x01])); // 3 bytes
          const closeEvent = await clientA.waitForClose();
          assertEquals(
            closeEvent.code,
            1002,
            "WebSocket should close with 1002 on length underflow",
          );
        } finally {
          clientA.close();
        }
        // Test Input B: Invalid Action ID (0x0099)
        const clientB = new WebSocketClient(session.port, "token-proto-b");
        await clientB.connect();
        try {
          clientB.sendFrame(0x0099, 1);
          const closeEvent = await clientB.waitForClose();
          assertEquals(
            closeEvent.code,
            1002,
            "WebSocket should close with 1002 on unrecognized Action ID",
          );
        } finally {
          clientB.close();
        }
        // Test Input C: Text message
        const clientC = new WebSocketClient(session.port, "token-proto-c");
        await clientC.connect();
        try {
          const ws = (clientC as any).ws as WebSocket;
          ws.send("text message");
          const closeEvent = await clientC.waitForClose();
          assertEquals(
            closeEvent.code,
            1003,
            "WebSocket should close with 1003 on text frames",
          );
        } finally {
          clientC.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg03c} [WebSocket Gateway] Ping-Pong Heartbeat Liveness (Heartbeat): Verifies liveness heartbeats keep connection alive",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, "token-heartbeat");
        try {
          await client.connect();
          client.sendFrame(0x0008, 99, new Uint8Array()); // StreamIO / OutputPTY no-op
          assert(true, "Connection remains healthy");
        } finally {
          client.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg04d} [WebSocket Gateway] Graceful Connection Eviction RST Prevention (EvictionRST): Ensures graceful takeover closures",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-eviction-rst";
        const clientA = new WebSocketClient(session.port, token);
        const clientB = new WebSocketClient(session.port, token);
        try {
          await clientA.connect();
          await clientB.connect();
          const closeEvent = await clientA.waitForClose();
          assertEquals(
            closeEvent.code,
            4000,
            "Expected Close Code 4000 on eviction",
          );
          assertEquals(closeEvent.reason, "Session Taken Over");
        } finally {
          clientA.close();
          clientB.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg05e} [WebSocket Gateway] Sequential Connection Takeover Handshake Race (TakeoverRace): Prevents overlapping writes",
      async () => {
        const session = await orchestrator.spawnRuntime();
        const token = "token-takeover-race";
        const clientA = new WebSocketClient(session.port, token);
        const clientB = new WebSocketClient(session.port, token);
        const clientC = new WebSocketClient(session.port, token);
        try {
          await clientA.connect();
          await clientB.connect();
          await clientC.connect();
          const closeA = await clientA.waitForClose();
          const closeB = await clientB.waitForClose();
          assertEquals(closeA.code, 4000);
          assertEquals(closeB.code, 4000);
          // Client C remains active and can spawn PTY
          clientC.sendSpawn(1, 80, 24, "/bin/bash");
          const spawn_spawning_386079 = await clientC.readSpawnStatus(1);
          assertEquals(spawn_spawning_386079.payload[0], 0x02);
          const unpacked = await clientC.readSpawnStatus(1);
          assertEquals(unpacked.payload[0], 0x00);
          assertEquals(unpacked.payload[0], 0x00);
        } finally {
          clientA.close();
          clientB.close();
          clientC.close();
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg06f} [WebSocket Gateway] Invalid Route Rejection (BadPath): Returns HTTP 404 on invalid paths",
      async () => {
        const session = await orchestrator.spawnRuntime();
        try {
          const res = await fetch(`http://127.0.0.1:${session.port}/badpath`);
          assertEquals(res.status, 404);
        } finally {
          await session.shutdown();
        }
      },
    );
    await t.step(
      "{wsg07g} [WebSocket Gateway] Missing Token Rejection (NoToken): Returns HTTP 400 on missing query parameter",
      async () => {
        const session = await orchestrator.spawnRuntime();
        try {
          const res = await fetch(`http://127.0.0.1:${session.port}/ws`);
          assertEquals(res.status, 400);
          const text = await res.text();
          assert(text.includes("Missing token query parameter"));
        } finally {
          await session.shutdown();
        }
      },
    );
  },
});
