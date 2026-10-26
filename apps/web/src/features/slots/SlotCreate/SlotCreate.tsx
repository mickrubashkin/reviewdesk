import { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Badge,
  Button,
  FileButton,
  Group,
  List,
  NumberInput,
  SegmentedControl,
  Select,
  Stack,
  Switch,
  Tabs,
  Text,
  Textarea,
  TextInput,
  Title,
} from "@mantine/core";
import { useDebouncedValue } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import { HouseIcon, UploadSimpleIcon } from "@phosphor-icons/react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";

import { useBlockCatalog, useDefaultLayout } from "../../blocks/api";
import { fetchBoards } from "../../boards/api";
import { createSlot, fetchEmails, inspectEmailHTML } from "../../emails/api";
import { formatStageName } from "../../emails/stages";
import { decidePortalGroup, reconstructPortalGroup, type PortalReconstruction } from "../../portal/api";
import type { AuthUser, EmailHTMLInspection, LayoutItem } from "../../emails/types";

import {
  effectivePreheader,
  effectiveSubject,
  emptyDraft,
  slotLanguages,
  slotWarnings,
  type LanguageDraft,
  type SlotLanguage,
} from "./slotLanguages";
import styles from "./SlotCreate.module.css";
import { BlocksSlotPanel, type BlocksDraft } from "./BlocksSlotPanel";
import { canManageBoard, managesAnyBoard } from "../../emails/permissions";

type PortalSource = { groupID: string; language: SlotLanguage | null; reconstruction: PortalReconstruction };

// SlotCreate optionally starts from emails sent on the portal
// (?portal_group=…): they are rebuilt on the board template first.
export function SlotCreate({ currentUser }: { currentUser: AuthUser }) {
  const [searchParams] = useSearchParams();
  const [groupIDs] = useState(() => searchParams.getAll("portal_group"));
  const queries = useQueries({
    queries: groupIDs.map((id) => ({
      queryKey: ["portal-reconstruct", id],
      queryFn: () => reconstructPortalGroup(id),
      staleTime: Infinity,
      retry: false,
    })),
  });

  if (queries.some((q) => q.isPending)) {
    return (
      <Stack align="center" justify="center" h="100dvh">
        <Text c="dimmed">Rebuilding the sent emails on the board template…</Text>
      </Stack>
    );
  }

  const sources: PortalSource[] = [];
  const initialDrafts = Object.fromEntries(slotLanguages.map((l) => [l, emptyDraft])) as Record<SlotLanguage, LanguageDraft>;
  const used = new Set<SlotLanguage>();
  queries.forEach((query, index) => {
    if (!query.data) return;
    const detected = query.data.language as SlotLanguage;
    const language =
      (slotLanguages as readonly string[]).includes(detected) && !used.has(detected)
        ? detected
        : slotLanguages.find((l) => !used.has(l)) ?? null;
    if (language) {
      used.add(language);
      initialDrafts[language] = { html: query.data.html, subject: query.data.subject || null, preheader: null };
    }
    sources.push({ groupID: groupIDs[index], language, reconstruction: query.data });
  });

  return (
    <SlotCreateForm
      currentUser={currentUser}
      failedSources={queries.filter((q) => q.isError).length}
      initialDrafts={initialDrafts}
      portalSources={sources}
    />
  );
}

function SlotCreateForm({
  currentUser,
  failedSources,
  initialDrafts,
  portalSources,
}: {
  currentUser: AuthUser;
  failedSources: number;
  initialDrafts: Record<SlotLanguage, LanguageDraft>;
  portalSources: PortalSource[];
}) {
  const canCreate = managesAnyBoard(currentUser);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [boardKey, setBoardKey] = useState<string | null>(null);
  const [stage, setStage] = useState<string | null>(
    () => portalSources.find((s) => s.reconstruction.suggested_stage)?.reconstruction.suggested_stage ?? null
  );
  const [title, setTitle] = useState("");
  const [autoPosition, setAutoPosition] = useState(true);
  const [sortOrder, setSortOrder] = useState<number | string>("");
  const [sendTiming, setSendTiming] = useState("");
  const [sendCondition, setSendCondition] = useState("");
  const [drafts, setDrafts] = useState<Record<SlotLanguage, LanguageDraft>>(initialDrafts);
  const [activeTab, setActiveTab] = useState<string | null>("en");
  const [error, setError] = useState<string | null>(null);
  // A new slot is built from blocks; pasted HTML is for exceptions and for
  // emails rebuilt from the portal.
  const [mode, setMode] = useState<"blocks" | "html">(portalSources.length > 0 ? "html" : "blocks");
  // null follows the usual blocks of the selected stage.
  const [editedLayout, setEditedLayout] = useState<LayoutItem[] | null>(null);
  const [blockLanguages, setBlockLanguages] = useState<SlotLanguage[]>(["en"]);
  const [blockDrafts, setBlockDrafts] = useState(
    () => Object.fromEntries(slotLanguages.map((l) => [l, { subject: "", preheader: "" }])) as Record<SlotLanguage, BlocksDraft>
  );

  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards, enabled: canCreate });
  const emailsQuery = useQuery({ queryKey: ["emails"], queryFn: () => fetchEmails(), enabled: canCreate });

  // Board managers create slots on their boards only.
  const boards = useMemo(
    () => (boardsQuery.data ?? []).filter((b) => canManageBoard(currentUser, b.key)),
    [boardsQuery.data, currentUser]
  );
  const selectedBoard =
    boards.find((b) => b.key === boardKey) ?? boards.find((b) => b.key === "onboarding") ?? boards[0];
  const selectedStage =
    stage && selectedBoard?.stages.includes(stage) ? stage : selectedBoard?.stages[0] ?? null;

  const emails = useMemo(() => emailsQuery.data ?? [], [emailsQuery.data]);
  const stageSlots = useMemo(() => {
    const bySortOrder = new Map<number, { title: string; sendTiming: string | null; languages: Set<string> }>();
    for (const email of emails) {
      if (email.sequence !== selectedBoard?.key || email.stage !== selectedStage) continue;
      const slot = bySortOrder.get(email.sort_order) ?? {
        title: email.title,
        sendTiming: email.send_timing,
        languages: new Set<string>(),
      };
      slot.languages.add(email.language);
      bySortOrder.set(email.sort_order, slot);
    }
    return [...bySortOrder.entries()].sort(([a], [b]) => a - b);
  }, [emails, selectedBoard?.key, selectedStage]);
  const timingOptions = useMemo(
    () => [...new Set(emails.map((e) => e.send_timing).filter((t): t is string => Boolean(t)))].sort(),
    [emails]
  );
  const blockCatalogQuery = useBlockCatalog(canCreate && mode === "blocks");
  const defaultLayoutQuery = useDefaultLayout(selectedBoard?.key, selectedStage, canCreate && mode === "blocks");
  const layout = editedLayout ?? defaultLayoutQuery.data;
  const positionTaken =
    !autoPosition && typeof sortOrder === "number" && stageSlots.some(([order]) => order === sortOrder);

  const htmlByLanguage = useMemo(
    () => Object.fromEntries(slotLanguages.map((l) => [l, drafts[l].html.trim()])) as Record<SlotLanguage, string>,
    [drafts]
  );
  const [debouncedHTML] = useDebouncedValue(htmlByLanguage, 500);
  const inspectionQueries = useQueries({
    queries: slotLanguages.map((language) => ({
      queryKey: ["email-html-inspection", debouncedHTML[language]],
      queryFn: () => inspectEmailHTML(debouncedHTML[language]),
      enabled: canCreate && debouncedHTML[language].length > 0,
      retry: false,
      staleTime: 30_000,
    })),
  });
  const inspections = Object.fromEntries(
    slotLanguages.map((l, i) => [l, htmlByLanguage[l] ? inspectionQueries[i].data : undefined])
  ) as Record<SlotLanguage, EmailHTMLInspection | undefined>;
  const inspectionFailed = Object.fromEntries(
    slotLanguages.map((l, i) => [l, Boolean(htmlByLanguage[l]) && inspectionQueries[i].isError])
  ) as Record<SlotLanguage, boolean>;

  const createMutation = useMutation({
    mutationFn: async (payload: Parameters<typeof createSlot>[0]) => {
      const response = await createSlot(payload);
      // Mark the portal emails this slot was made from, so they leave the review list.
      await Promise.all(
        portalSources.map((source) => {
          const created = response.emails.find((e) => e.language === source.language);
          return created
            ? decidePortalGroup(source.groupID, { decision: "created", email_id: created.id })
            : Promise.resolve();
        })
      );
      return response;
    },
    onSuccess: (response) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: `${response.emails.length} language(s) created at position ${response.sort_order}.`,
        title: "Slot created",
      });
      const master = response.emails.find((e) => e.language === "en") ?? response.emails[0];
      navigate(`/emails/${encodeURIComponent(master.id)}/review`);
    },
    onError: (err) => setError(err instanceof Error ? err.message : "Create failed"),
  });

  if (!canCreate) {
    return (
      <Stack align="center" justify="center" h="100dvh">
        <Alert color="red" title="Admin access required">
          You do not have permission to create slots.
        </Alert>
        <Button component={Link} to="/" variant="light">
          Home
        </Button>
      </Stack>
    );
  }

  const filledLanguages = mode === "blocks" ? blockLanguages : slotLanguages.filter((l) => htmlByLanguage[l]);
  const contentReady =
    mode === "blocks"
      ? Boolean(layout && layout.length > 0)
      : htmlByLanguage.en.length > 0 && !filledLanguages.some((l) => inspectionFailed[l]);
  const canSubmit =
    Boolean(selectedBoard && selectedStage) &&
    title.trim().length > 0 &&
    contentReady &&
    !positionTaken &&
    (autoPosition || typeof sortOrder === "number") &&
    !createMutation.isPending;

  const updateDraft = (language: SlotLanguage, patch: Partial<LanguageDraft>) =>
    setDrafts((current) => ({ ...current, [language]: { ...current[language], ...patch } }));

  const submit = () => {
    if (!canSubmit || !selectedBoard || !selectedStage) return;
    setError(null);
    const slot = {
      sequence: selectedBoard.key,
      stage: selectedStage,
      title: title.trim(),
      sort_order: autoPosition ? undefined : (sortOrder as number),
      send_timing: sendTiming.trim() || undefined,
      send_condition: sendCondition.trim() || undefined,
    };
    if (mode === "blocks") {
      createMutation.mutate({
        ...slot,
        layout,
        emails: blockLanguages.map((language) => ({
          language,
          subject: blockDrafts[language].subject.trim() || undefined,
          preheader: blockDrafts[language].preheader.trim() || undefined,
        })),
      });
      return;
    }
    createMutation.mutate({
      ...slot,
      emails: filledLanguages.map((language) => ({
        language,
        original_html: htmlByLanguage[language],
        subject: effectiveSubject(drafts[language], inspections[language]).trim() || undefined,
        preheader: effectivePreheader(drafts[language], inspections[language]).trim() || undefined,
      })),
    });
  };

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Button component={Link} leftSection={<HouseIcon aria-hidden="true" size={16} />} size="xs" to="/" variant="subtle">
          Home
        </Button>
        <Stack gap={0}>
          <Title order={4}>New slot</Title>
          <Text c="dimmed" size="xs">
            One position in the sequence, with all its languages. EN is the source.
          </Text>
        </Stack>
      </header>

      <main className={styles.content}>
        <section className={styles.panel}>
          <Stack>
            <Select
              allowDeselect={false}
              data={boards.map((b) => ({ label: b.name, value: b.key }))}
              label="Board"
              value={selectedBoard?.key ?? null}
              onChange={setBoardKey}
            />
            <Select
              allowDeselect={false}
              data={(selectedBoard?.stages ?? []).map((s) => ({ label: formatStageName(s), value: s }))}
              label="Stage"
              value={selectedStage}
              onChange={setStage}
            />
            <TextInput
              description="Same for all languages, e.g. Second Follow-up"
              label="Title"
              required
              value={title}
              onChange={(e) => setTitle(e.currentTarget.value)}
            />
            <Autocomplete
              data={timingOptions}
              description="Delay as set in the robot"
              label="Send timing"
              placeholder="+2 days"
              value={sendTiming}
              onChange={setSendTiming}
            />
            <TextInput
              description="Robot condition, if any"
              label="Send condition"
              placeholder="If documents are not uploaded"
              value={sendCondition}
              onChange={(e) => setSendCondition(e.currentTarget.value)}
            />
            <Switch
              checked={autoPosition}
              label="Add at the end of the stage"
              onChange={(e) => setAutoPosition(e.currentTarget.checked)}
            />
            {!autoPosition ? (
              <NumberInput
                error={positionTaken ? "This position is already used" : undefined}
                label="Position (sort order)"
                min={0}
                value={sortOrder}
                onChange={setSortOrder}
              />
            ) : null}
            <Stack gap={4}>
              <Text fw={600} size="sm">
                Already in this stage
              </Text>
              {stageSlots.length === 0 ? (
                <Text c="dimmed" size="xs">
                  No emails yet.
                </Text>
              ) : (
                stageSlots.map(([order, slot]) => (
                  <Text key={order} size="xs">
                    <Text c="dimmed" component="span" size="xs">
                      #{order}
                    </Text>{" "}
                    {slot.title} · {slot.sendTiming ?? "no timing"} ·{" "}
                    {[...slot.languages].sort().join(", ").toUpperCase()}
                  </Text>
                ))
              )}
            </Stack>

            {portalSources.length > 0 || failedSources > 0 ? (
              <Alert color="blue" title="From emails sent on the portal">
                {(
                  <List size="sm">
                    {portalSources.map((source) => (
                      <List.Item key={source.groupID}>
                        {(source.language ?? "?").toUpperCase()}: {source.reconstruction.from_template
                          ? `rebuilt on the template of "${source.reconstruction.template_email_title}"`
                          : "kept its own structure"}
                        {source.reconstruction.warnings.map((w) => (
                          <Text key={w} c="orange" size="xs">
                            {w}
                          </Text>
                        ))}
                      </List.Item>
                    ))}
                    {failedSources > 0 ? (
                      <List.Item>Some emails could not be rebuilt.</List.Item>
                    ) : null}
                  </List>
                )}
              </Alert>
            ) : null}
            {error ? (
              <Alert color="red" title="Could not create the slot">
                {error}
              </Alert>
            ) : null}
            <Button disabled={!canSubmit} loading={createMutation.isPending} onClick={submit}>
              Create slot ({filledLanguages.length} {filledLanguages.length === 1 ? "language" : "languages"})
            </Button>
          </Stack>
        </section>

        <section className={styles.panel}>
          <SegmentedControl
            data={[
              { label: "From blocks", value: "blocks" },
              { label: "Paste HTML", value: "html" },
            ]}
            mb="md"
            value={mode}
            onChange={(value) => setMode(value as "blocks" | "html")}
          />
          {mode === "blocks" && selectedBoard ? (
            <BlocksSlotPanel
              board={selectedBoard.key}
              catalog={blockCatalogQuery.data ?? []}
              drafts={blockDrafts}
              isDefault={editedLayout === null}
              languages={blockLanguages}
              layout={layout}
              layoutError={defaultLayoutQuery.isError}
              onDraftChange={(language, patch) =>
                setBlockDrafts((current) => ({ ...current, [language]: { ...current[language], ...patch } }))
              }
              onLanguagesChange={setBlockLanguages}
              onLayoutChange={setEditedLayout}
              onLayoutReset={() => setEditedLayout(null)}
            />
          ) : null}
          {mode === "html" ? (
            <Tabs value={activeTab} onChange={setActiveTab}>
              <Tabs.List>
                {slotLanguages.map((language) => (
                  <Tabs.Tab
                    key={language}
                    rightSection={<LanguageStatus filled={Boolean(htmlByLanguage[language])} warnings={tabWarningCount(language)} failed={inspectionFailed[language]} />}
                    value={language}
                  >
                    {language.toUpperCase()}
                    {language === "en" ? " *" : ""}
                  </Tabs.Tab>
                ))}
              </Tabs.List>
              {slotLanguages.map((language) => (
                <Tabs.Panel key={language} pt="md" value={language}>
                  <LanguagePanel
                    draft={drafts[language]}
                    failed={inspectionFailed[language]}
                    inspection={inspections[language]}
                    language={language}
                    slotWarnings={slotWarnings(language, inspections[language], inspections.en)}
                    onChange={(patch) => updateDraft(language, patch)}
                  />
                </Tabs.Panel>
              ))}
            </Tabs>
          ) : null}
        </section>
      </main>
    </div>
  );

  function tabWarningCount(language: SlotLanguage) {
    const inspection = inspections[language];
    if (!inspection) return 0;
    return inspection.warnings.length + slotWarnings(language, inspection, inspections.en).length;
  }
}

function LanguageStatus({ filled, warnings, failed }: { filled: boolean; warnings: number; failed: boolean }) {
  if (!filled) return null;
  if (failed) return <Badge color="red" size="xs">!</Badge>;
  return warnings > 0 ? <Badge color="yellow" size="xs">{warnings}</Badge> : <Badge color="green" size="xs">ok</Badge>;
}

function LanguagePanel({
  draft,
  failed,
  inspection,
  language,
  slotWarnings: extraWarnings,
  onChange,
}: {
  draft: LanguageDraft;
  failed: boolean;
  inspection: EmailHTMLInspection | undefined;
  language: SlotLanguage;
  slotWarnings: string[];
  onChange: (patch: Partial<LanguageDraft>) => void;
}) {
  const warnings = [...extraWarnings, ...(inspection?.warnings ?? [])];

  return (
    <Stack>
      <Group justify="space-between">
        <Text c="dimmed" size="sm">
          {language === "en"
            ? "Required. Paste the HTML from the CRM robot."
            : "Optional. Leave empty if this language is not ready."}
        </Text>
        <FileButton accept="text/html,.html,.htm" onChange={(file) => void file?.text().then((html) => onChange({ html }))}>
          {(props) => (
            <Button {...props} leftSection={<UploadSimpleIcon aria-hidden="true" size={14} />} size="xs" variant="light">
              Load .html file
            </Button>
          )}
        </FileButton>
      </Group>
      <Textarea
        autosize
        className={styles.htmlInput}
        label="HTML"
        maxRows={10}
        minRows={4}
        placeholder="<!doctype html>…"
        value={draft.html}
        onChange={(e) => onChange({ html: e.currentTarget.value })}
      />

      {failed ? <Alert color="red">This HTML could not be parsed or has conflicting field values.</Alert> : null}

      {draft.html.trim() ? (
        <>
          <Group grow>
            <TextInput
              description={draft.subject === null ? "Taken from <title>" : undefined}
              label="Subject"
              value={effectiveSubject(draft, inspection)}
              onChange={(e) => onChange({ subject: e.currentTarget.value })}
            />
            <TextInput
              description={draft.preheader === null ? "Taken from the HTML" : undefined}
              label="Preheader"
              value={effectivePreheader(draft, inspection)}
              onChange={(e) => onChange({ preheader: e.currentTarget.value })}
            />
          </Group>
          {inspection ? (
            <Group gap="xs">
              <Badge variant="light">{inspection.editable_field_count} editable fields</Badge>
              <Badge variant="light">{inspection.review_block_count} review blocks</Badge>
              {inspection.detection.bitrix_expressions.map((expression) => (
                <Badge key={expression} color="grape" variant="light">
                  keeps {expression}
                </Badge>
              ))}
            </Group>
          ) : null}
          {warnings.length > 0 ? (
            <Alert color="yellow" title="Check before creating">
              <List size="sm">
                {warnings.map((warning) => (
                  <List.Item key={warning}>{warning}</List.Item>
                ))}
              </List>
            </Alert>
          ) : null}
          {inspection ? (
            <iframe
              className={styles.previewFrame}
              sandbox=""
              srcDoc={inspection.review_html}
              title={`${language.toUpperCase()} preview`}
            />
          ) : null}
        </>
      ) : null}
    </Stack>
  );
}
