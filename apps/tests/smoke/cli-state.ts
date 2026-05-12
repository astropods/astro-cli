import { execSync, type ExecSyncOptionsWithStringEncoding, type ExecSyncOptionsWithBufferEncoding } from "child_process";

export const CLI_STATE_FILE = ".playwright-cli-state.json";

type ExecOpts = (ExecSyncOptionsWithStringEncoding | ExecSyncOptionsWithBufferEncoding) & {
  retries?: number;
};

export function exec(cmd: string, opts: ExecSyncOptionsWithStringEncoding & { retries?: number }): string;
export function exec(cmd: string, opts?: ExecSyncOptionsWithBufferEncoding & { retries?: number }): Buffer;
export function exec(cmd: string, opts?: ExecOpts): string | Buffer {
  const { retries = 0, ...execOpts } = opts ?? {};
  console.log("$", cmd);
  let lastErr: unknown;
  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      return execSync(cmd, execOpts as Parameters<typeof execSync>[1]);
    } catch (err) {
      lastErr = err;
      if (attempt < retries) {
        console.log(`retrying (${attempt + 1}/${retries})...`);
      }
    }
  }
  throw lastErr;
}
