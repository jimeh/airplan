import { execFile } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execFileAsync = promisify(execFile);
const here = dirname(fileURLToPath(import.meta.url));
const windows = process.platform === 'win32';

/** Repository root, resolved from this module's own location. */
export const repoRoot = join(here, '..', '..');

/**
 * Path of the airplan binary the browser tests drive. The global setup
 * builds it once so no test hook pays compile time; `bin/` is ignored by
 * Git, and the name is distinct from `mise run build`'s `bin/airplan` so
 * neither can serve the other a stale binary.
 */
export const binaryPath = join(
  repoRoot,
  'bin',
  windows ? 'airplan-browser.exe' : 'airplan-browser',
);

/**
 * Environment with every AIRPLAN_* variable removed, so ambient
 * configuration cannot leak into the fixtures the tests render.
 */
export function cleanEnv() {
  return Object.fromEntries(
    Object.entries(process.env).filter(
      ([name]) => !name.startsWith('AIRPLAN_'),
    ),
  );
}

/** Compile the airplan binary that the browser tests drive. */
export async function buildAirplan() {
  const env = cleanEnv();
  // Ignore the developer's persisted `go env -w` settings. A stored
  // GOOS, GOARCH, or GOFLAGS would otherwise produce a binary the tests
  // cannot execute; the hook used to get this isolation for free by
  // pointing XDG_CONFIG_HOME at its temporary fixture root.
  env.GOENV = 'off';

  // Resolve the toolchain through the shim, then build with that binary
  // directly, so the compile uses the Go version mise selects for this
  // worktree rather than whatever else PATH resolution may find.
  const { stdout: goRoot } = await execFileAsync(
    'go', ['env', 'GOROOT'], { cwd: repoRoot, env },
  );
  const goPath = join(goRoot.trim(), 'bin', windows ? 'go.exe' : 'go');

  // -o to an explicit path: a bare "go build ." would try to write a
  // binary named airplan over the airplan/ package directory.
  await execFileAsync(
    goPath, ['build', '-o', binaryPath, '.'], { cwd: repoRoot, env },
  );
}
