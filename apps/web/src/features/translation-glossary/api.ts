import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";

export const glossaryLanguages = [
  { code: "de", label: "DE" },
  { code: "es", label: "ES" },
  { code: "pl", label: "PL" },
  { code: "br", label: "BR" },
] as const;

// An English term the AI pre-translation must handle strictly. A language
// without a translation keeps the English term. Shared by all boards.
export type GlossaryTerm = {
  term: string;
  translations: Partial<Record<string, string>>;
  note: string;
};

type GlossaryPayload = { terms: GlossaryTerm[] };

const glossaryURL = "/api/translation-glossary";

export const glossaryQueryKey = ["translation-glossary"];

export async function fetchGlossary(): Promise<GlossaryTerm[]> {
  const response = await fetchJson<GlossaryPayload>(glossaryURL);
  return response.terms;
}

export async function saveGlossary(terms: GlossaryTerm[]): Promise<GlossaryTerm[]> {
  const response = await fetchJson<GlossaryPayload>(glossaryURL, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ terms }),
  });
  return response.terms;
}

export function useGlossary() {
  return useQuery({
    queryKey: glossaryQueryKey,
    queryFn: fetchGlossary,
    staleTime: 60_000,
  });
}
