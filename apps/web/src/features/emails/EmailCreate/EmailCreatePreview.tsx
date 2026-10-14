import { Badge, Group, Stack, Text } from "@mantine/core";

import type { CreateEmailFormState } from "./EmailCreate.types";
import styles from "./EmailCreate.module.css";

export function EmailCreatePreview({
  formState,
  previewHTML,
  trimmedHTML,
}: {
  formState: CreateEmailFormState;
  previewHTML: string;
  trimmedHTML: string;
}) {
  return (
    <>
      <Group className={styles.previewHeader} justify="space-between">
        <Stack gap={0}>
          <Text fw={600} size="sm">
            Preview
          </Text>
          <Text c="dimmed" size="xs">
            Raw uploaded HTML
          </Text>
        </Stack>
        <Group gap={6}>
          <Badge variant="light">{formState.language.toUpperCase() || "EN"}</Badge>
          <Badge variant="light">{formState.variant || "v1"}</Badge>
          <Badge variant="light">{formState.adaptationLabel || "Default"}</Badge>
        </Group>
      </Group>
      <div className={styles.previewFrameWrap}>
        {trimmedHTML ? (
          <iframe
            className={styles.previewFrame}
            sandbox=""
            srcDoc={previewHTML}
            title="New email preview"
          />
        ) : (
          <Stack align="center" justify="center" h="100%">
            <Text c="dimmed" size="sm">
              Upload or paste HTML to preview it.
            </Text>
          </Stack>
        )}
      </div>
    </>
  );
}
