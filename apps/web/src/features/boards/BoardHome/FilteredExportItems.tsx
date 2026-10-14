import { Menu } from "@mantine/core";

import type { FilteredExport } from "./useFilteredExport";

export function FilteredExportItems({
  filteredExport,
}: {
  filteredExport: FilteredExport;
}) {
  const disabled = filteredExport.emailIds.length === 0 || filteredExport.isPending;

  return (
    <>
      <Menu.Label>Export {filteredExport.emailIds.length} filtered emails</Menu.Label>
      <Menu.Item disabled={disabled} onClick={filteredExport.exportHTML}>
        HTML (zip)
      </Menu.Item>
      <Menu.Item disabled={disabled} onClick={filteredExport.openMarkdownDialog}>
        Markdown for AI (zip)…
      </Menu.Item>
    </>
  );
}
