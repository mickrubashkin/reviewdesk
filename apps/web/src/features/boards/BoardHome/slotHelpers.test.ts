import { describe, expect, it } from "vitest";

import { cellVersions } from "./slotHelpers";

const version = (id: string, variant: string, fields: Partial<Parameters<typeof cellVersions>[0][number]> = {}) => ({
  id,
  variant,
  adaptation_key: "default",
  adaptation_label: "Default",
  production_status: "not_seen" as const,
  portal_send_count: 0,
  portal_sends_30d: 0,
  portal_last_sent_at: null,
  portal_quiet_days: null,
  ...fields,
});

describe("cellVersions", () => {
  it("puts the shown version first and says which ones go out", () => {
    const rows = cellVersions(
      [
        version("old", "old", { production_status: "live", portal_send_count: 40, portal_sends_30d: 11 }),
        version("new", "new", { production_status: "live", portal_send_count: 10, portal_sends_30d: 3 }),
        version("manager", "new", { adaptation_key: "mena", adaptation_label: "MENA" }),
      ],
      "new"
    );
    expect(rows.map((row) => [row.label, row.sends, row.live])).toEqual([
      ["new", "live · 3/30d", true],
      ["old", "live · 11/30d", true],
      ["new · MENA", "not sent", false],
    ]);
  });

  it("tells a version that stopped from one never sent", () => {
    const [row] = cellVersions(
      [version("old", "old", { portal_send_count: 5, portal_last_sent_at: "2026-08-01T10:00:00Z" })],
      "old"
    );
    expect(row.sends).toMatch(/^not sent lately, last /);
  });
});
