export function topStages(stages: Record<string, number>, names: Map<string, string>) {
  const entries = Object.entries(stages ?? {}).sort(([, a], [, b]) => b - a);
  if (entries.length === 0) return "—";
  return entries
    .slice(0, 2)
    .map(([id, count]) => `${names.get(id) ?? id}${entries.length > 1 ? ` (${count})` : ""}`)
    .join(", ");
}
