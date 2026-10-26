import { Badge, Button, Group, Paper, Stack, Text, Title, UnstyledButton } from "@mantine/core";
import { useNavigate } from "react-router-dom";

import {
  commentLink,
  useMarkNotificationsRead,
  type AppNotification,
} from "../../comments/notifications";
import { formatStageName } from "../../emails/stages";
import { formatCommentDate } from "../../review/EmailReview/EmailReview.helpers";
import { useUserName } from "../../../shared/userNames";
import styles from "./MyTasks.module.css";

// MentionList shows the comments that mention me and are not read yet.
// Opening one marks it read.
export function MentionList({
  boardNames,
  notifications,
}: {
  boardNames: Map<string, string>;
  notifications: AppNotification[];
}) {
  const userName = useUserName();
  const navigate = useNavigate();
  const markRead = useMarkNotificationsRead();

  return (
    <Stack gap="xs">
      <Group gap="xs" justify="space-between">
        <Group gap="xs">
          <Title order={4}>Mentions</Title>
          <Badge color={notifications.length > 0 ? "blue" : "gray"} variant="light">
            {notifications.length}
          </Badge>
        </Group>
        {notifications.length > 1 ? (
          <Button loading={markRead.isPending} size="compact-xs" variant="subtle" onClick={() => markRead.mutate([])}>
            Mark all read
          </Button>
        ) : null}
      </Group>
      <Text c="dimmed" size="xs">
        Comments where someone wrote @ and your name.
      </Text>
      {notifications.length === 0 ? (
        <Text c="dimmed" size="sm">
          Nothing here.
        </Text>
      ) : (
        <Paper withBorder>
          {notifications.map((notification) => (
            <UnstyledButton
              key={notification.id}
              className={styles.row}
              onClick={() => {
                markRead.mutate([notification.id]);
                navigate(commentLink(notification.email_id, notification.comment_id));
              }}
            >
              <Group gap="sm" justify="space-between" wrap="nowrap" align="flex-start">
                <Group gap="sm" wrap="nowrap" miw={0} align="flex-start">
                  <Badge color="gray" size="sm" variant="outline">
                    {notification.email_language.toUpperCase()}
                  </Badge>
                  <Stack gap={0} miw={0}>
                    <Text size="sm" truncate>
                      <Text component="span" fw={600} inherit>
                        {userName(notification.actor_email, "Someone")}
                      </Text>{" "}
                      on {notification.email_title}
                    </Text>
                    <Text c="dimmed" size="xs" truncate>
                      {boardNames.get(notification.board_key) ?? notification.board_key} ·{" "}
                      {formatStageName(notification.email_stage ?? "")} · {notification.email_variant}
                    </Text>
                    <Text lineClamp={2} size="sm">
                      {notification.excerpt}
                    </Text>
                  </Stack>
                </Group>
                <Text c="dimmed" size="xs" style={{ whiteSpace: "nowrap" }}>
                  {formatCommentDate(notification.created_at)}
                </Text>
              </Group>
            </UnstyledButton>
          ))}
        </Paper>
      )}
    </Stack>
  );
}
