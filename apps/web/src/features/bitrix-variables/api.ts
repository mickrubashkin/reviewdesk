import { useQuery } from "@tanstack/react-query";

import { fetchJson } from "../../shared/api";

// A Bitrix24 expression the CRM robot fills in when it sends an email, e.g.
// the responsible manager's booking link. Shared by all boards.
export type BitrixVariable = {
  label: string;
  expression: string;
  example: string;
  note: string;
};

type BitrixVariablesPayload = { variables: BitrixVariable[] };

const variablesURL = "/api/bitrix-variables";

export const bitrixVariablesQueryKey = ["bitrix-variables"];

export async function fetchBitrixVariables(): Promise<BitrixVariable[]> {
  const response = await fetchJson<BitrixVariablesPayload>(variablesURL);
  return response.variables;
}

export async function saveBitrixVariables(
  variables: BitrixVariable[]
): Promise<BitrixVariable[]> {
  const response = await fetchJson<BitrixVariablesPayload>(variablesURL, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ variables }),
  });
  return response.variables;
}

export function useBitrixVariables() {
  return useQuery({
    queryKey: bitrixVariablesQueryKey,
    queryFn: fetchBitrixVariables,
    staleTime: 60_000,
  });
}
