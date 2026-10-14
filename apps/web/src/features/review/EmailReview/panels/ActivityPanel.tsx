import { Alert, Badge, Group, Loader, Stack, Text, Timeline } from "@mantine/core";

import type { EmailActivityItem } from "../../../emails/types";
import {
  activityColor,
  activityDetail,
  activitySummary,
  formatActivityType,
  formatCommentDate,
} from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";

export function ActivityPanel({
  activities,
  isError,
  isLoading,
}: {
  activities: EmailActivityItem[];
  isError: boolean;
  isLoading: boolean;
}) {
  if (isLoading) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center">
        <Loader size="sm" />
        <Text c="dimmed" size="sm">
          Loading activity
        </Text>
      </Stack>
    );
  }

  if (isError) {
    return (
      <Alert color="red" title="Failed to load activity">
        Activity is unavailable right now.
      </Alert>
    );
  }

  if (activities.length === 0) {
    return (
      <Stack className={styles.emptyState} align="center" justify="center" gap="xs">
        <Text fw={600}>No activity yet</Text>
        <Text c="dimmed" ta="center" size="sm">
          Review actions will appear here as this email changes.
        </Text>
      </Stack>
    );
  }

  return (
    <Timeline active={activities.length} bulletSize={24} lineWidth={2}>
      {activities.map((activity) => (
        <Timeline.Item
          color={activityColor(activity.type)}
          key={activity.id}
          title={
            <Group justify="space-between" gap="xs" wrap="nowrap">
              <Stack gap={2}>
                <Text fw={700} size="xs">
                  {activity.actor_email ?? "System"}
                </Text>
                <Text c="dimmed" size="xs">
                  {formatCommentDate(activity.created_at)}
                </Text>
              </Stack>
              <Badge color={activityColor(activity.type)} size="sm" variant="light">
                {formatActivityType(activity)}
              </Badge>
            </Group>
          }
        >
          <Stack className={styles.activityItem} gap={6}>
            <Text size="sm">{activitySummary(activity)}</Text>
            {activityDetail(activity) ? (
              <Text c="dimmed" size="xs">
                {activityDetail(activity)}
              </Text>
            ) : null}
          </Stack>
        </Timeline.Item>
      ))}
    </Timeline>
  );
}
