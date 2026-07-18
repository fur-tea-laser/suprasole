import { join as joinPath } from "@std/path";

export interface ServerSession {
  port: number;
  process: Deno.ChildProcess;
  shutdown: () => Promise<void>;
}

export class ServerOrchestrator {
  private binaryPath: string;

  constructor() {
    this.binaryPath = joinPath(import.meta.dirname!, "..", "suprasole-server");
  }

  async spawnRuntime(args: string[] = []): Promise<ServerSession> {
    let retries = 5;
    while (retries > 0) {
      const command = new Deno.Command(this.binaryPath, {
        args: ["-port", "0", ...args],
        stdout: "piped",
        stderr: "inherit",
      });
      const process = command.spawn();
      const reader = process.stdout.getReader();
      const decoder = new TextDecoder();
      let booted = false;
      let actualPort = 0;
      let buffer = "";
      try {
        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value);
          const match = buffer.match(/Server listening on port (\d+)/);
          if (match) {
            booted = true;
            actualPort = parseInt(match[1], 10);
            break;
          }
        }
      } catch {
        // Ignore read/boot errors
      } finally {
        try {
          await reader.cancel();
        } catch {
          // Ignore
        }
      }
      if (booted) {
        const shutdown = async () => {
          try {
            // Retrieve and kill all child processes (e.g. spawned shells) of the Go server
            const pgrep = new Deno.Command("pgrep", {
              args: ["-P", process.pid.toString()],
            });
            const output = await pgrep.output();
            const pids = new TextDecoder().decode(output.stdout).trim().split(
              /\s+/,
            ).filter(Boolean);
            for (const pid of pids) {
              try {
                const k = new Deno.Command("kill", { args: ["-9", pid] });
                await k.output();
              } catch {
                // Ignore
              }
            }
          } catch {
            // Ignore
          }
          try {
            process.kill("SIGKILL");
            await process.status;
          } catch {
            // Ignore
          }
        };
        return { port: actualPort, process, shutdown };
      } else {
        retries--;
        try {
          process.kill("SIGKILL");
          await process.status;
        } catch {
          // Ignore
        }
      }
    }
    throw new Error("Server failed to boot after 5 port retries");
  }
}
