import { buildAirplan } from './airplan-binary.js';

/**
 * Build the airplan binary once for the whole run.
 *
 * Compiling inside a test hook instead makes every project repeat the
 * build, and a cold Go module and build cache — as on a dependency bump
 * that changes go.sum — overruns the per-hook timeout. Global setup has
 * no such deadline.
 */
export default async function globalSetup() {
  await buildAirplan();
}
