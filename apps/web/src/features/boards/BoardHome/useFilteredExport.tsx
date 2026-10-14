import { useState } from "react";
import { Button, Checkbox, Group, Modal, Stack } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation } from "@tanstack/react-query";

import { downloadEmailsExport } from "../../emails/api";
import {
  allMarkdownSections,
  markdownSectionOptions,
} from "../../emails/markdownSections";

type ExportRequest = {
  format: "html" | "md";
  markdownOptions?: { sections: string[]; openCommentsOnly: boolean };
};

export type FilteredExport = {
  emailIds: string[];
  isPending: boolean;
  exportHTML: () => void;
  openMarkdownDialog: () => void;
  dialog: React.ReactNode;
};

// State lives outside the filter menus: their dropdowns unmount on close,
// which would otherwise tear down the dialog and any in-flight export.
export function useFilteredExport(emailIds: string[]): FilteredExport {
  const [dialogOpened, setDialogOpened] = useState(false);
  const [sections, setSections] = useState(allMarkdownSections);
  const [openCommentsOnly, setOpenCommentsOnly] = useState(false);
  const exportMutation = useMutation({
    mutationFn: ({ format, markdownOptions }: ExportRequest) =>
      downloadEmailsExport(emailIds, format, markdownOptions),
    onSuccess: ({ blob, fileName }) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");

      link.href = url;
      link.download = fileName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setDialogOpened(false);
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Export failed",
      });
    },
  });

  const dialog = (
    <Modal
      opened={dialogOpened}
      title={`Export ${emailIds.length} emails as Markdown`}
      onClose={() => setDialogOpened(false)}
    >
      <Stack>
        <Checkbox.Group value={sections} onChange={setSections}>
          <Stack gap="xs">
            {markdownSectionOptions.map((option) => (
              <Checkbox key={option.value} label={option.label} value={option.value} />
            ))}
          </Stack>
        </Checkbox.Group>
        <Checkbox
          checked={openCommentsOnly}
          disabled={!sections.includes("comments")}
          label="Only open comments"
          ml="md"
          onChange={(event) => setOpenCommentsOnly(event.currentTarget.checked)}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={() => setDialogOpened(false)}>
            Cancel
          </Button>
          <Button
            disabled={emailIds.length === 0 || sections.length === 0}
            loading={exportMutation.isPending}
            onClick={() =>
              exportMutation.mutate({
                format: "md",
                markdownOptions: { openCommentsOnly, sections },
              })
            }
          >
            Export
          </Button>
        </Group>
      </Stack>
    </Modal>
  );

  return {
    dialog,
    emailIds,
    exportHTML: () => exportMutation.mutate({ format: "html" }),
    isPending: exportMutation.isPending,
    openMarkdownDialog: () => setDialogOpened(true),
  };
}
