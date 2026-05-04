import { execSync, type ExecSyncOptionsWithStringEncoding, type ExecSyncOptionsWithBufferEncoding } from "child_process";

export const CLI_STATE_FILE = ".playwright-cli-state.json";

export function exec(cmd: string, opts: ExecSyncOptionsWithStringEncoding): string;
export function exec(cmd: string, opts?: ExecSyncOptionsWithBufferEncoding): Buffer;
export function exec(cmd: string, opts?: Parameters<typeof execSync>[1]): string | Buffer {
  console.log("$", cmd);
  return execSync(cmd, opts as Parameters<typeof execSync>[1]);
}
