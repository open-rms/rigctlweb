import { error } from "@sveltejs/kit";
import { doc, docs } from "#lib/docs.ts";

export const entries = () => docs.map((d) => ({ slug: d.slug }));

export function load({ params }) {
  const d = doc(params.slug);
  if (!d) error(404, "No such page");
  return { doc: d };
}
