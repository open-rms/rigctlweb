import { latestRelease } from "#lib/release.ts";

export async function load({ fetch }) {
  return { release: await latestRelease(fetch) };
}
