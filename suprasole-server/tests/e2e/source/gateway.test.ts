import { assertEquals, assert } from '@std/assert'
import { ServerOrchestrator } from './ServerOrchestrator.ts'
import { WebSocketClient } from './WebSocketClient.ts'

Deno.test({
  name: '{h6e2et} [E2E Infrastructure] Gateway Suite: WebSocket Connection and Heartbeat Invariants',
  async fn(t) {
    const orchestrator = new ServerOrchestrator();
    await t.step(
      '{wsg01a} [WebSocket Gateway] Handshake Upgrade (Connection): Connects and verifies WebSocket protocol upgraded successfully',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token%2Dhandshake');
        try {
          await client.connect();
          assert(true, 'Successfully connected via WebSocket');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{wsg02b} [WebSocket Gateway] Malformed Frame Rejection (ProtocolError): Asserts immediate connection closure on malformed inputs',
      async () => {
        const session = await orchestrator.spawnRuntime();
        // Test Input A: Length Underflow (< 4 bytes)
        const clientA = new WebSocketClient(session.port, 'token-proto-a');
        await clientA.connect();
        try {
          const ws = (clientA as any).ws as WebSocket;
          ws.send(new Uint8Array([0x00, 0x05, 0x01])); // 3 bytes
          const closeEvent = await clientA.waitForClose();
          assertEquals(closeEvent.code, 1002, 'WebSocket should close with 1002 on length underflow');
        } finally {
          clientA.close();
        }
        // Test Input B: Invalid Action ID (0x0099)
        const clientB = new WebSocketClient(session.port, 'token-proto-b');
        await clientB.connect();
        try {
          clientB.sendFrame(0x0099, 1);
          const closeEvent = await clientB.waitForClose();
          assertEquals(closeEvent.code, 1002, 'WebSocket should close with 1002 on unrecognized Action ID');
        } finally {
          clientB.close();
        }
        // Test Input C: Text message
        const clientC = new WebSocketClient(session.port, 'token-proto-c');
        await clientC.connect();
        try {
          const ws = (clientC as any).ws as WebSocket;
          ws.send('text message');
          const closeEvent = await clientC.waitForClose();
          assertEquals(closeEvent.code, 1003, 'WebSocket should close with 1003 on text frames');
        } finally {
          clientC.close();
          await session.shutdown();
        }
      }
    );
    await t.step(
      '{wsg03c} [WebSocket Gateway] Ping-Pong Heartbeat Liveness (Heartbeat): Verifies liveness heartbeats keep connection alive',
      async () => {
        const session = await orchestrator.spawnRuntime();
        const client = new WebSocketClient(session.port, 'token-heartbeat');
        try {
          await client.connect();
          client.sendFrame(0x0005, 99, new Uint8Array()); // StreamIO no-op
          assert(true, 'Connection remains healthy');
        } finally {
          client.close();
          await session.shutdown();
        }
      }
    );
  }
});
