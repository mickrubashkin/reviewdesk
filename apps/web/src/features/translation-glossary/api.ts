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

// A term someone proposed for the glossary; a super admin accepts or
// rejects it. language and translation are empty for a bare English term.
export type GlossarySuggestion = {
  id: string;
  term: string;
  language: string;
  translation: string;
  note: string;
  email_id: string | null;
  email_title: string | null;
  suggested_by_email: string;
  status: "pending" | "accepted" | "rejected";
  decided_by_email: string | null;
  decided_at: string | null;
  created_at: string;
};

export type GlossarySuggestionInput = {
  term: string;
  language: string;
  translation: string;
  note: string;
  email_id?: string | null;
};

const suggestionsURL = `${glossaryURL}/suggestions`;

export const glossarySuggestionsQueryKey = ["translation-glossary", "suggestions"];

// A super admin gets the pending suggestions; anyone else their own.
export function useGlossarySuggestions() {
  return useQuery({
    queryKey: glossarySuggestionsQueryKey,
    queryFn: () => fetchJson<GlossarySuggestion[]>(suggestionsURL),
  });
}

export function suggestGlossaryTerm(input: GlossarySuggestionInput): Promise<GlossarySuggestion> {
  return fetchJson<GlossarySuggestion>(suggestionsURL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function acceptGlossarySuggestion(id: string, input: GlossarySuggestionInput): Promise<GlossaryTerm[]> {
  const response = await fetchJson<GlossaryPayload>(`${suggestionsURL}/${encodeURIComponent(id)}/accept`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  return response.terms;
}

export function rejectGlossarySuggestion(id: string): Promise<void> {
  return fetchJson<void>(`${suggestionsURL}/${encodeURIComponent(id)}/reject`, { method: "POST" });
}
