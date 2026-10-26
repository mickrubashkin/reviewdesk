import { Alert, Button, Checkbox, Group, Loader, Modal, Stack, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { CheckIcon, CopyIcon } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError } from "../../shared/api";
import { fetchBoards } from "../boards/api";
import { fetchRenderedEmail, markEmailUploaded } from "./api";
import { robotsBehind, uploadRobots } from "./reviewStatus";
import { formatStageName } from "./stages";
import type { EmailListItem } from "./types";

type BitrixCopyModalProps = {
  email: EmailListItem | null;
  // Set when the dialog walks a queue: after a mark it moves on instead of closing.
  nextEmail?: EmailListItem | null;
  opened: boolean;
  onClose: () => void;
  onNext?: (email: EmailListItem) => void;
};

// BitrixCopyModal is the hand-off to the Bitrix24 robots: it shows where the
// email goes, copies its HTML and records the copied version as uploaded to
// the robots it was pasted into.
export function BitrixCopyModal({ email, nextEmail, opened, onClose, onNext }: BitrixCopyModalProps) {
  return (
    <Modal opened={opened && email !== null} size="md" title="Copy for Bitrix24" onClose={onClose}>
      {email ? (
        <BitrixCopyContent
          // A new email starts with nothing copied.
          key={email.id}
          email={email}
          nextEmail={nextEmail ?? null}
          onClose={onClose}
          onNext={onNext}
        />
      ) : null}
    </Modal>
  );
}

function BitrixCopyContent({
  email,
  nextEmail,
  onClose,
  onNext,
}: {
  email: EmailListItem;
  nextEmail: EmailListItem | null;
  onClose: () => void;
  onNext?: (email: EmailListItem) => void;
}) {
  const queryClient = useQueryClient();
  const [copiedVersion, setCopiedVersion] = useState<number | null>(null);
  const renderedQuery = useQuery({
    queryKey: ["emails", email.id, "rendered"],
    queryFn: () => fetchRenderedEmail(email.id),
    refetchOnMount: "always",
  });
  const rendered = renderedQuery.data;
  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards });
  const boardName = boardsQuery.data?.find((board) => board.key === email.sequence)?.name;
  // The rendered response is fresh; the list entry may predate an edit.
  const isReady = (rendered?.review_status ?? email.review_status) === "ready";
  const subject = rendered?.subject ?? email.subject;
  const robots = uploadRobots(email);
  // A region with managers has a robot each; pick the ones pasted.
  const named = robots.length > 1 || robots[0]?.robot !== "";
  const robotHas = (version: number | null | undefined) =>
    new Set(robots.filter((robot) => version != null && robot.version === version).map((robot) => robot.robot));
  const [chosen, setChosen] = useState<string[]>(() => robotsBehind(email).map((robot) => robot.robot));
  const current = robotHas(rendered?.version);
  const toMark = named ? chosen.filter((robot) => !current.has(robot)) : robots.map((robot) => robot.robot);
  const alreadyUploaded = rendered?.version != null && robots.every((robot) => current.has(robot.robot));

  const markMutation = useMutation({
    mutationFn: (version: number) => markEmailUploaded(email.id, true, version, toMark),
    onSuccess: (_, version) => {
      const into = named ? ` in ${toMark.join(", ")}'s ${toMark.length === 1 ? "robot" : "robots"}` : "";
      notifications.show({ color: "green", message: `Version ${version} is marked uploaded${into}.`, title: "Uploaded" });
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", email.id] });
      if (nextEmail && onNext) {
        onNext(nextEmail);
      } else {
        onClose();
      }
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) {
        // The email changed or went back to draft: refresh the queue too.
        setCopiedVersion(null);
        void renderedQuery.refetch();
        void queryClient.invalidateQueries({ queryKey: ["emails"] });
      }
      notifications.show({ color: "red", message: error.message, title: "Not marked" });
    },
  });

  const copy = async () => {
    if (!rendered) {
      return;
    }
    try {
      await navigator.clipboard.writeText(rendered.html);
      setCopiedVersion(rendered.version);
    } catch {
      notifications.show({ color: "red", message: "Browser blocked clipboard access.", title: "Copy failed" });
    }
  };

  const place = [
    boardName ?? email.sequence,
    email.stage ? formatStageName(email.stage) : null,
    email.language.toUpperCase(),
    // new and old variants of one slot both sit in robots: tell them apart.
    email.variant,
    email.adaptation_key !== "default" ? email.adaptation_label : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <Stack gap="md">
      <Stack gap={4}>
        <Text fw={600}>{email.title}</Text>
        <Text c="dimmed" size="sm">
          {place}
        </Text>
        {email.send_timing ? <Text size="sm">When: {email.send_timing}</Text> : null}
        {email.send_condition ? <Text size="sm">Condition: {email.send_condition}</Text> : null}
        {subject ? <Text size="sm">Subject: {subject}</Text> : null}
      </Stack>

      {renderedQuery.isLoading ? <Loader size="sm" /> : null}
      {renderedQuery.isError ? (
        <Alert color="red" variant="light">
          Failed to render the HTML.
        </Alert>
      ) : null}

      {rendered ? (
        <Text size="sm">
          {rendered.version != null ? `Version ${rendered.version}` : "No saved version yet"}
          {alreadyUploaded
            ? " · Bitrix24 already has it"
            : !named
              ? email.uploaded_version != null
                ? ` · Bitrix24 has version ${email.uploaded_version}`
                : " · not in Bitrix24 yet"
              : ""}
        </Text>
      ) : null}

      {named ? (
        <Checkbox.Group
          label="Pasted into the robots of"
          size="sm"
          // Robots that have this version show as done.
          value={[...chosen.filter((robot) => !current.has(robot)), ...current]}
          onChange={(value) => setChosen(value.filter((robot) => !current.has(robot)))}
        >
          <Stack gap={6} mt={6}>
            {robots.map((robot) => (
              <Checkbox
                key={robot.robot}
                disabled={current.has(robot.robot)}
                label={`${robot.robot} · ${
                  robot.version === null
                    ? "not uploaded"
                    : current.has(robot.robot)
                      ? `has version ${robot.version}`
                      : `has version ${robot.version}, older`
                }`}
                value={robot.robot}
              />
            ))}
          </Stack>
        </Checkbox.Group>
      ) : null}

      {!isReady ? (
        <Alert color="gray" variant="light">
          The email is a draft. You can copy it, but mark it Ready before recording the upload.
        </Alert>
      ) : null}

      <Group gap="xs" grow>
        <Button
          disabled={!rendered}
          leftSection={<CopyIcon aria-hidden="true" size={15} />}
          variant={copiedVersion === null ? "filled" : "light"}
          onClick={() => void copy()}
        >
          {copiedVersion === null ? "Copy HTML" : "Copy again"}
        </Button>
        <Button
          disabled={copiedVersion === null || !isReady || alreadyUploaded || toMark.length === 0}
          leftSection={<CheckIcon aria-hidden="true" size={15} />}
          loading={markMutation.isPending}
          variant={copiedVersion === null ? "light" : "filled"}
          onClick={() => copiedVersion !== null && markMutation.mutate(copiedVersion)}
        >
          {copiedVersion === null ? "Mark uploaded" : `Mark v${copiedVersion} uploaded`}
        </Button>
      </Group>
      <Text c="dimmed" size="xs">
        Paste the HTML into the email of {named ? "each robot" : "the robot"}, save it, then mark it uploaded.
        {nextEmail && onNext ? ` Next in the queue: ${nextEmail.title} (${nextEmail.language.toUpperCase()}).` : ""}
      </Text>
    </Stack>
  );
}
