import { execFile } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
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

export const fixtureBinaryPath = join(
  repoRoot,
  'bin',
  windows ? 'airplan-browser-fixture.exe' : 'airplan-browser-fixture',
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
  // cannot execute. The hook used to get this on Linux as a side effect
  // of pointing XDG_CONFIG_HOME at its temporary fixture root, which Go
  // consults for the env file only on Unix. Explicit process variables
  // still win, so an exported GOPROXY or GOMODCACHE keeps working.
  env.GOENV = 'off';

  // Ask the toolchain where it lives, then build through that binary
  // rather than PATH. Both resolve to the same Go; going direct only
  // keeps a mise shim from mutating the build environment on the way.
  const { stdout: goRoot } = await execFileAsync(
    'go', ['env', 'GOROOT'], { cwd: repoRoot, env },
  );
  const goPath = join(goRoot.trim(), 'bin', windows ? 'go.exe' : 'go');

  // `go build -o` does create missing parents, but `go help build`
  // promises only that it writes to the named file or directory, so
  // create bin/ rather than depend on undocumented behaviour.
  await mkdir(dirname(binaryPath), { recursive: true });

  // -o to an explicit path: a bare "go build ." would try to write a
  // binary named airplan over the airplan/ package directory.
  await execFileAsync(
    goPath, ['build', '-o', binaryPath, '.'], { cwd: repoRoot, env },
  );
  await execFileAsync(
    goPath,
    ['build', '-o', fixtureBinaryPath, './tests/browser/fixture'],
    { cwd: repoRoot, env },
  );
}
