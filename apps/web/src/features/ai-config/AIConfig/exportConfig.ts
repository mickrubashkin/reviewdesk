import type { BoardAIConfigDraft } from "../api";

export function configToMarkdown(boardKey: string, draft: BoardAIConfigDraft) {
  const lines = [`# AI review setup: ${boardKey}`, ""];

  lines.push("## Instruction", "", draft.instruction || "—", "");
  lines.push("## Roles", "");
  for (const role of draft.roles) {
    lines.push(`- ${role.enabled ? "" : "(disabled) "}**${role.name}**: ${role.description}`);
  }
  if (draft.roles.length === 0) lines.push("—");
  lines.push("", "## Rules", "");
  for (const rule of draft.rules) {
    lines.push(`- ${rule.enabled ? "" : "(disabled) "}**${rule.title}**: ${rule.body}`);
  }
  if (draft.rules.length === 0) lines.push("—");
  lines.push("", "## Sequence context", "", draft.sequence_context || "—", "");

  return lines.join("\n");
}

export function downloadTextFile(content: string, fileName: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement("a");

  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

// Accepts a previously exported JSON file; anything else is rejected.
export function parseConfigJSON(text: string): BoardAIConfigDraft {
  const raw = JSON.parse(text) as Partial<BoardAIConfigDraft>;
  if (typeof raw !== "object" || raw === null) {
    throw new Error("Not a config object");
  }

  return {
    instruction: String(raw.instruction ?? ""),
    sequence_context: String(raw.sequence_context ?? ""),
    rules: (Array.isArray(raw.rules) ? raw.rules : []).map((rule) => ({
      title: String(rule?.title ?? ""),
      body: String(rule?.body ?? ""),
      enabled: rule?.enabled !== false,
    })),
    roles: (Array.isArray(raw.roles) ? raw.roles : []).map((role) => ({
      name: String(role?.name ?? ""),
      description: String(role?.description ?? ""),
      enabled: role?.enabled !== false,
    })),
  };
}
