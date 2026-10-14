import { Group, Skeleton, Stack } from "@mantine/core";
import styles from "../EmailReview.module.css";

export function ReviewPreviewSkeleton() {
  return (
    <div className={styles.previewSkeleton}>
      <Stack gap="md">
        <Group gap="sm" wrap="nowrap">
          <Skeleton circle h={40} w={40} />
          <Stack gap={6} flex={1}>
            <Skeleton h={12} w={140} />
            <Skeleton h={12} w="60%" />
            <Skeleton h={10} w={180} />
          </Stack>
        </Group>
        <Skeleton h={220} radius="md" />
        <Stack gap="sm">
          <Skeleton h={14} w="42%" />
          <Skeleton h={14} w="72%" />
          <Skeleton h={14} w="68%" />
          <Skeleton h={14} w="58%" />
        </Stack>
      </Stack>
    </div>
  );
}
