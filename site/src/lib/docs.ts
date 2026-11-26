// The docs live in the repository's docs/ folder (readable on GitHub as well); this
// renders them at build time. Front matter: title, description, order. Files without front
// matter (ROADMAP, DONATING) are for the repository only and are left out.
import { Marked } from "marked";

const GITHUB = "https://github.com/open-rms/rigctlweb";

const files = import.meta.glob("../../../docs/*.md", { query: "?raw", import: "default", eager: true }) as Record<string, string>;

export type Doc = { slug: string; file: string; title: string; description: string; order: number; html: string; headings: { id: string; text: string }[] };

function slugify(s: string): string {
  return s.toLowerCase().replace(/<[^>]+>/g, "").replace(/[^a-z0-9 -]/g, "").trim().replace(/\s+/g, "-");
}

function parse(file: string, raw: string): Doc | null {
  const slug = file.replace(/\.md$/, "").toLowerCase();
  let meta: Record<string, string> = {};
  let body = raw;
  const m = raw.match(/^---\n([\s\S]*?)\n---\n/);
  if (m) {
    for (const line of m[1].split("\n")) {
      const i = line.indexOf(":");
      if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    }
    body = raw.slice(m[0].length);
  } else {
    return null;
  }
  const headings: { id: string; text: string }[] = [];
  const marked = new Marked({
    gfm: true,
    renderer: {
      heading({ tokens, depth }) {
        const text = this.parser.parseInline(tokens);
        const id = slugify(text);
        if (depth === 2) headings.push({ id, text: text.replace(/<[^>]+>/g, "") });
        // the page shows the title itself
        if (depth === 1) return "";
        return `<h${depth} id="${id}"><a class="anchor" href="#${id}">${text}</a></h${depth}>\n`;
      },
      link({ href, title, tokens }) {
        const text = this.parser.parseInline(tokens);
        // docs/foo.md#bar -> /docs/foo/#bar; external links open in place but are marked
        let h = href;
        const md = href.match(/^([A-Za-z0-9-]+)\.md(#.*)?$/);
        if (md) h = files[`../../../docs/${md[1]}.md`]?.startsWith("---\n") ? `/docs/${md[1].toLowerCase()}/${md[2] ?? ""}` : `${GITHUB}/blob/main/docs/${md[1]}.md${md[2] ?? ""}`;
        // repository files outside docs/ (../SECURITY.md, openapi.json) open on GitHub
        else if (!/^(https?:|#|\/)/.test(href)) h = `${GITHUB}/blob/main/docs/${href}`;
        const ext = /^https?:/.test(h);
        const t = title ? ` title="${title}"` : "";
        return `<a href="${h}"${t}${ext ? ' rel="noopener"' : ""}>${text}</a>`;
      },
    },
  });
  const html = marked.parse(body) as string;
  return {
    slug,
    file,
    title: meta.title ?? slug,
    description: meta.description ?? "",
    order: Number(meta.order ?? 99),
    html,
    headings,
  };
}

export const docs: Doc[] = Object.entries(files)
  .map(([path, raw]) => parse(path.split("/").pop()!, raw))
  .filter((d): d is Doc => d !== null)
  .sort((a, b) => a.order - b.order);

export function doc(slug: string): Doc | undefined {
  return docs.find((d) => d.slug === slug);
}
