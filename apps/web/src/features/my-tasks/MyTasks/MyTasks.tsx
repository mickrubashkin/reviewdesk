import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Paper,
  Stack,
  Text,
  Title,
  Tooltip,
  UnstyledButton,
} from "@mantine/core";
import { HouseIcon } from "@phosphor-icons/react";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { fetchBoards } from "../../boards/api";
import { takeEmail } from "../../emails/api";
import { BitrixCopyModal } from "../../emails/BitrixCopyModal";

import { formatStageName } from "../../emails/stages";
import type { AuthUser, EmailListItem } from "../../emails/types";
import { UploadBadge } from "../../emails/UploadBadge";
import type { UploadSuggestion } from "../../portal/api";
import { MarkSeenUploadedButton } from "../../portal/MarkSeenUploadedButton";
import { describeSuggestion, suggestionFor } from "../../portal/uploadSuggestions";
import { isMine, isTakenByOther, useMyTasks, type TaskSection } from "../tasks";
import { MentionList } from "./MentionList";
import styles from "./MyTasks.module.css";
import { useUserName } from "../../../shared/userNames";

export function MyTasks({ currentUser }: { currentUser: AuthUser }) {
  const { emailsQuery, mentions, sections, suggestions, count } = useMyTasks(currentUser);
  const hasLanguages = (currentUser.languages ?? []).length > 0;
  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards });
  const boardNames = new Map((boardsQuery.data ?? []).map((board) => [board.key, board.name]));

  return (
    <div className={styles.page}>
      <Stack className={styles.content} gap="lg">
        <Group justify="space-between" wrap="nowrap">
          <Stack gap={2}>
            <Title order={2}>My tasks</Title>
            <Text c="dimmed" size="sm">
              {emailsQuery.isLoading
                ? "Loading…"
                : count === 0
                  ? "Nothing waiting for you."
                  : `${count} ${count === 1 ? "thing needs" : "things need"} your attention.`}
            </Text>
          </Stack>
          <Button component={Link} leftSection={<HouseIcon aria-hidden="true" size={16} />} to="/" variant="light">
            Home
          </Button>
        </Group>

        {!hasLanguages ? (
          <Alert color="blue" variant="light">
            No translation languages are set for you, so outdated translations only show for emails you own. An admin
            can set them on the Users page.
          </Alert>
        ) : null}

        <MentionList boardNames={boardNames} notifications={mentions} />

        {emailsQuery.isLoading ? <Loader /> : null}
        {emailsQuery.isError ? <Alert color="red">Failed to load emails.</Alert> : null}

        {sections.map((section) => (
          <TaskSectionList
            key={section.key}
            boardNames={boardNames}
            currentUser={currentUser}
            section={section}
            suggestions={suggestions}
          />
        ))}
      </Stack>
    </div>
  );
}

function TaskSectionList({
  boardNames,
  currentUser,
  section,
  suggestions,
}: {
  boardNames: Map<string, string>;
  currentUser: AuthUser;
  section: TaskSection;
  suggestions: Map<string, UploadSuggestion>;
}) {
  // The Bitrix24 upload queue opens one email at a time and moves on after each mark.
  const [copyEmail, setCopyEmail] = useState<EmailListItem | null>(null);
  const copyIndex = copyEmail ? section.emails.findIndex((email) => email.id === copyEmail.id) : -1;
  const nextEmail = copyIndex >= 0 ? (section.emails[copyIndex + 1] ?? null) : null;
  return (
    <Stack gap="xs">
      <Group gap="xs">
        <Title order={4}>{section.title}</Title>
        <Badge color={section.emails.length > 0 ? "blue" : "gray"} variant="light">
          {section.emails.length}
        </Badge>
      </Group>
      <Text c="dimmed" size="xs">
        {section.hint}
      </Text>
      {section.emails.length === 0 ? (
        <Text c="dimmed" size="sm">
          Nothing here.
        </Text>
      ) : (
        <Paper withBorder>
          {section.emails.map((email) => (
            <TaskRow
              key={email.id}
              boardName={boardNames.get(email.sequence) ?? email.sequence}
              currentUser={currentUser}
              email={email}
              section={section}
              suggestion={section.key === "seen" ? suggestionFor(suggestions, email) : null}
              onCopy={setCopyEmail}
            />
          ))}
        </Paper>
      )}
      {section.key === "upload" ? (
        <BitrixCopyModal
          email={copyEmail}
          nextEmail={nextEmail}
          opened={copyEmail !== null}
          onClose={() => setCopyEmail(null)}
          onNext={setCopyEmail}
        />
      ) : null}
    </Stack>
  );
}

function TaskRow({
  boardName,
  currentUser,
  email,
  section,
  suggestion,
  onCopy,
}: {
  boardName: string;
  currentUser: AuthUser;
  email: EmailListItem;
  section: TaskSection;
  suggestion: UploadSuggestion | null;
  onCopy: (email: EmailListItem) => void;
}) {
  const userName = useUserName();
  const navigate = useNavigate();
  const takenByOther = isTakenByOther(email, currentUser);
  return (
    <UnstyledButton
      className={styles.row}
      data-dimmed={(section.key === "outdated" && takenByOther) || undefined}
      onClick={() => navigate(`/emails/${encodeURIComponent(email.id)}/review`)}
    >
      <Group gap="sm" justify="space-between" wrap="nowrap">
        <Group gap="sm" wrap="nowrap" miw={0}>
          <Badge color="gray" size="sm" variant="outline">
            {email.language.toUpperCase()}
          </Badge>
          <Stack gap={0} miw={0}>
            <Text fw={600} size="sm" truncate>
              {email.title}
            </Text>
            <Text c="dimmed" size="xs" truncate>
              {boardName} · {formatStageName(email.stage ?? "")} · {email.variant}
              {email.adaptation_key !== "default" ? ` · ${email.adaptation_label}` : ""}
              {email.subject ? ` · ${email.subject}` : ""}
            </Text>
          </Stack>
        </Group>
        <Group gap={4} wrap="nowrap">
          {section.key === "comments" ? (
            <Badge color="orange" size="sm" variant="light">
              {email.open_comment_count} open
            </Badge>
          ) : null}
          {section.key === "seen" && suggestion ? (
            <>
              <Tooltip label={describeSuggestion(suggestion)}>
                <Badge color="teal" size="sm" variant="light">
                  Portal sends v{suggestion.version}
                </Badge>
              </Tooltip>
              <MarkSeenUploadedButton email={email} suggestion={suggestion} />
            </>
          ) : null}
          {section.key === "upload" ? (
            <>
              <UploadBadge email={email} />
              <Button
                component="span"
                size="compact-xs"
                variant="light"
                onClick={(event) => {
                  event.stopPropagation();
                  onCopy(email);
                }}
              >
                Copy for Bitrix24
              </Button>
            </>
          ) : null}
          {section.key === "outdated" ? (
            <>
              <Badge color="red" size="sm" variant="light">
                EN changed
              </Badge>
              {takenByOther ? (
                <Badge color="gray" size="sm" variant="light">
                  Taken by {userName(email.owner_email)}
                </Badge>
              ) : (
                <TakeButton email={email} mine={isMine(email, currentUser)} />
              )}
            </>
          ) : null}
        </Group>
      </Group>
    </UnstyledButton>
  );
}

function TakeButton({ email, mine }: { email: EmailListItem; mine: boolean }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => takeEmail(email.id, !mine),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["emails"] }),
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });
  return (
    <Button
      component="span"
      loading={mutation.isPending}
      size="compact-xs"
      variant={mine ? "subtle" : "light"}
      onClick={(event) => {
        event.stopPropagation();
        mutation.mutate();
      }}
    >
      {mine ? "Give back" : "Take"}
    </Button>
  );
}
