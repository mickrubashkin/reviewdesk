import { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Drawer,
  Group,
  Loader,
  Select,
  SimpleGrid,
  Stack,
  Tabs,
  Text,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery } from "@tanstack/react-query";
import { diffWords } from "diff";
import { useNavigate } from "react-router-dom";

import { fetchEmails } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import { decidePortalGroup, fetchPortalGroup, type GroupDecision } from "../api";
import { topStages } from "./portalHelpers";

export function GroupDrawer({
  boardKey,
  groupID,
  stageNames,
  onClose,
  onDecided,
}: {
  boardKey: string;
  groupID: string;
  stageNames: Map<string, string>;
  onClose: () => void;
  onDecided: () => void;
}) {
  const [chosenEmailID, setChosenEmailID] = useState<string | null>(null);
  const navigate = useNavigate();
  const detailQuery = useQuery({
    queryKey: ["portal-group", groupID, chosenEmailID],
    queryFn: () => fetchPortalGroup(groupID, chosenEmailID ?? undefined),
  });
  const emailsQuery = useQuery({ queryKey: ["emails", boardKey], queryFn: () => fetchEmails(boardKey) });
  const decideMutation = useMutation({
    mutationFn: (payload: { decision: GroupDecision | null; email_id?: string }) =>
      decidePortalGroup(groupID, payload),
    onSuccess: () => {
      onDecided();
      onClose();
    },
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
  });

  const detail = detailQuery.data;
  const language = detail?.language;
  const candidates = (emailsQuery.data ?? [])
    .filter((e) => e.variant !== "old" && (!language || e.language === language))
    .map((e) => ({
      label: `${formatStageName(e.stage ?? "")} #${e.sort_order} · ${e.title} · ${e.language.toUpperCase()} · ${e.adaptation_label}`,
      value: e.id,
    }));
  const parts = detail?.match ? diffWords(detail.match.text, detail.sample_text) : [];

  return (
    <Drawer opened position="right" size="90%" title={detail?.subject ?? "Sent email"} onClose={onClose}>
      {!detail ? (
        <Loader />
      ) : (
        <Stack>
          <Group gap="xs">
            <Badge variant="light">{detail.language.toUpperCase() || "language ?"}</Badge>
            <Badge variant="light">{detail.send_count} sends</Badge>
            <Text size="sm">
              {new Date(detail.first_sent_at).toLocaleDateString()} – {new Date(detail.last_sent_at).toLocaleDateString()}
            </Text>
            <Text c="dimmed" size="sm">
              Portal stage: {topStages(detail.stages, stageNames)}
            </Text>
          </Group>

          <Select
            clearable
            data={candidates}
            description="The closest board email is picked automatically; choose another if it is wrong."
            label="Compare with"
            searchable
            value={chosenEmailID ?? detail.match?.id ?? null}
            onChange={setChosenEmailID}
          />

          {detail.match ? (
            <Tabs defaultValue="diff">
              <Tabs.List>
                <Tabs.Tab value="diff">Text differences</Tabs.Tab>
                <Tabs.Tab value="preview">Side by side</Tabs.Tab>
              </Tabs.List>
              <Tabs.Panel pt="md" value="diff">
                <Text c="dimmed" mb="xs" size="xs">
                  <span style={{ background: "var(--mantine-color-green-1)" }}>Green</span> is only in the sent email,{" "}
                  <span style={{ background: "var(--mantine-color-red-1)", textDecoration: "line-through" }}>red</span>{" "}
                  only in the service.
                </Text>
                <Text size="sm" style={{ lineHeight: 1.7 }}>
                  {parts.map((part, index) => (
                    <span
                      key={index}
                      style={
                        part.added
                          ? { background: "var(--mantine-color-green-1)" }
                          : part.removed
                            ? { background: "var(--mantine-color-red-1)", textDecoration: "line-through" }
                            : undefined
                      }
                    >
                      {part.value}
                    </span>
                  ))}
                </Text>
              </Tabs.Panel>
              <Tabs.Panel pt="md" value="preview">
                <SimpleGrid cols={2}>
                  <Preview html={detail.sample_html} label="Sent (from the deal timeline)" />
                  <Preview html={detail.match.html} label={`In the service: ${detail.match.title}`} />
                </SimpleGrid>
              </Tabs.Panel>
            </Tabs>
          ) : (
            <>
              <Alert color="orange">No similar email in the service. It can become a new slot.</Alert>
              <Preview html={detail.sample_html} label="Sent (from the deal timeline)" />
            </>
          )}

          <Group>
            <Button
              disabled={!detail.match}
              loading={decideMutation.isPending}
              onClick={() => detail.match && decideMutation.mutate({ decision: "confirmed", email_id: detail.match.id })}
            >
              This is the email in the service
            </Button>
            <Button
              variant="light"
              onClick={() => navigate(`/slots/new?portal_group=${encodeURIComponent(groupID)}`)}
            >
              Create a new slot from it
            </Button>
            <Button color="gray" variant="light" onClick={() => decideMutation.mutate({ decision: "ignored" })}>
              Ignore (test, one-off)
            </Button>
            {detail.decision ? (
              <Button color="red" variant="subtle" onClick={() => decideMutation.mutate({ decision: null })}>
                Clear decision ({detail.decision})
              </Button>
            ) : null}
          </Group>
        </Stack>
      )}
    </Drawer>
  );
}

function Preview({ html, label }: { html: string; label: string }) {
  return (
    <Stack gap={4}>
      <Text fw={600} size="sm">
        {label}
      </Text>
      <iframe
        sandbox=""
        srcDoc={html}
        style={{ width: "100%", height: 640, border: "1px solid var(--mantine-color-gray-3)" }}
        title={label}
      />
    </Stack>
  );
}
