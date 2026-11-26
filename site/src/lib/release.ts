// The download table, filled in at build time from the newest GitHub release. A rebuild picks
// up a new tag.
export const REPO = "open-rms/rigctlweb";
export const GITHUB_URL = `https://github.com/${REPO}`;

export type Asset = { name: string; url: string; size: number };
export type Download = { label: string; detail: string; assets: Asset[] };
export type Release = { tag: string; url: string; published: string; prerelease: boolean; downloads: Download[]; sums?: Asset };

const groups: { label: string; detail: string; test: RegExp }[] = [
  { label: "Raspberry Pi and other 64-bit ARM Linux", detail: "chmod +x, then run it (or see the install guide for a service)", test: /-linux-arm64$/ },
  { label: "Linux on a PC", detail: "chmod +x, then run it", test: /-linux-amd64$/ },
  { label: "macOS", detail: "chmod +x, then run it from Terminal", test: /-darwin-(arm64|amd64)$/ },
  { label: "Windows", detail: "run it from a terminal", test: /-windows-amd64\.exe$/ },
];

const ARCH_ORDER = ["arm64", "amd64"];
function byArch(a: Asset, b: Asset): number {
  const k = (n: string) => ARCH_ORDER.findIndex((x) => n.includes(x));
  return k(a.name) - k(b.name);
}

export async function latestRelease(fetchFn: typeof fetch): Promise<Release | null> {
  try {
    const r = await fetchFn(`https://api.github.com/repos/${REPO}/releases?per_page=5`, {
      headers: { accept: "application/vnd.github+json", "user-agent": "rigctlweb-site" },
    });
    if (!r.ok) return null;
    const list = (await r.json()) as any[];
    const rel = list.find((x) => !x.draft && x.assets?.length);
    if (!rel) return null;
    const assets: Asset[] = rel.assets.map((a: any) => ({ name: a.name, url: a.browser_download_url, size: a.size }));
    return {
      tag: rel.tag_name,
      url: rel.html_url,
      published: rel.published_at,
      prerelease: rel.prerelease,
      sums: assets.find((a) => a.name === "sha256sums.txt"),
      downloads: groups
        .map((g) => ({ label: g.label, detail: g.detail, assets: assets.filter((a) => g.test.test(a.name)).sort(byArch) }))
        .filter((g) => g.assets.length),
    };
  } catch {
    return null;
  }
}

/** A button label for a file: "arm64", "x86-64", "Apple silicon"... */
export function archOf(name: string): string {
  if (name.includes("darwin-arm64")) return "Apple silicon";
  if (name.includes("darwin-amd64")) return "Intel";
  if (name.includes("arm64")) return "arm64 (aarch64)";
  if (name.includes("amd64")) return "x86-64";
  return name;
}

export function mb(n: number): string {
  return `${(n / 1048576).toFixed(n > 20971520 ? 0 : 1)} MB`;
}
