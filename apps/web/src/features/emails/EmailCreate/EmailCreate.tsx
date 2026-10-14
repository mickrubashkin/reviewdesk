import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Stack,
  Tabs,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";

import { fetchBoards } from "../../boards/api";
import { createEmail, fetchEmails, inspectEmailHTML } from "../api";
import type { AuthUser, CreateEmailPayload } from "../types";
import {
  buildEventGroupOptions,
  getNextSortOrder,
  optionalString,
  uniqueSorted,
} from "./EmailCreate.helpers";
import { EmailCreateFormPanel } from "./EmailCreateFormPanel";
import { EmailCreateHeader } from "./EmailCreateHeader";
import { EmailCreatePreview } from "./EmailCreatePreview";
import {
  initialFormState,
  newEventGroupValue,
  type CreateEmailFormState,
} from "./EmailCreate.types";
import styles from "./EmailCreate.module.css";

type EmailCreateProps = {
  currentUserRole: AuthUser["role"];
};

export function EmailCreate({ currentUserRole }: EmailCreateProps) {
  const canCreate =
    currentUserRole === "admin" || currentUserRole === "super_admin";
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [formState, setFormState] = useState<CreateEmailFormState>(initialFormState);
  const [createError, setCreateError] = useState<string | null>(null);
  const [debouncedHTML, setDebouncedHTML] = useState("");
  const [sortOrderAuto, setSortOrderAuto] = useState(true);
  const emailsQuery = useQuery({
    queryKey: ["emails"],
    queryFn: () => fetchEmails(),
    enabled: canCreate,
  });
  const boardsQuery = useQuery({
    queryKey: ["boards"],
    queryFn: fetchBoards,
    enabled: canCreate,
  });
  const htmlInspectionQuery = useQuery({
    queryKey: ["email-html-inspection", debouncedHTML],
    queryFn: () => inspectEmailHTML(debouncedHTML),
    enabled: canCreate && debouncedHTML.trim().length > 0,
    retry: false,
    staleTime: 30_000,
  });
  const createMutation = useMutation({
    mutationFn: (payload: CreateEmailPayload) => createEmail(payload),
    onSuccess: (createdEmail) => {
      setCreateError(null);
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      queryClient.setQueryData(["emails", createdEmail.id, "review"], createdEmail);
      notifications.show({
        color: "green",
        message: "Email is ready for review.",
        title: "Email created",
      });
      navigate(`/emails/${encodeURIComponent(createdEmail.id)}/review`);
    },
    onError: (error) => {
      setCreateError(error instanceof Error ? error.message : "Create failed");
    },
  });

  const emails = useMemo(() => emailsQuery.data ?? [], [emailsQuery.data]);
  const boards = useMemo(() => boardsQuery.data ?? [], [boardsQuery.data]);
  const boardOptions = useMemo(
    () => boards.map((board) => ({ label: board.name, value: board.key })),
    [boards]
  );
  const boardKeys = useMemo(() => boards.map((board) => board.key), [boards]);
  const selectedBoard = boardKeys.includes(formState.sequence)
    ? formState.sequence
    : boards.find((board) => board.key === "onboarding")?.key ?? boardKeys[0] ?? "";
  const activeBoard = boards.find((board) => board.key === selectedBoard);
  const stageOptions = activeBoard?.stages ?? [];
  const selectedStage = stageOptions.includes(formState.stage)
    ? formState.stage
    : stageOptions[0] ?? "";
  const suggestedSortOrder = getNextSortOrder(
    emails,
    selectedBoard,
    selectedStage
  );
  const selectedSortOrder = sortOrderAuto
    ? suggestedSortOrder
    : formState.sortOrder;
  const languageOptions = useMemo(
    () => uniqueSorted(emails.map((email) => email.language)),
    [emails]
  );
  const variantOptions = useMemo(
    () => uniqueSorted(emails.map((email) => email.variant)),
    [emails]
  );
  const adaptationOptions = useMemo(
    () => uniqueSorted(["Default", ...emails.map((email) => email.adaptation_label)]),
    [emails]
  );
  const eventGroupOptions = useMemo(
    () => buildEventGroupOptions(emails, selectedBoard, selectedStage),
    [emails, selectedBoard, selectedStage]
  );
  const eventSelectOptions = useMemo(
    () => [
      { label: "Create new event", value: newEventGroupValue },
      ...eventGroupOptions,
    ],
    [eventGroupOptions]
  );
  const selectedEventGroup = eventGroupOptions.find(
    (option) => option.value === formState.eventGroupKey
  );
  useEffect(() => {
    const timeoutID = window.setTimeout(() => {
      setDebouncedHTML(formState.originalHTML.trim());
    }, 500);

    return () => window.clearTimeout(timeoutID);
  }, [formState.originalHTML]);

  if (!canCreate) {
    return (
      <div className={styles.page}>
        <Stack align="center" justify="center" h="100dvh">
          <Alert color="red" title="Admin access required">
            You do not have permission to create emails.
          </Alert>
          <Button component={Link} to="/" variant="light">
            Home
          </Button>
        </Stack>
      </div>
    );
  }

  const trimmedTitle = formState.title.trim();
  const trimmedStage = selectedStage.trim();
  const trimmedHTML = formState.originalHTML.trim();
  const htmlInspection = htmlInspectionQuery.data;
  const htmlInspectionWarnings = htmlInspection?.warnings ?? [];
  const previewHTML = htmlInspection?.review_html ?? trimmedHTML;
  const canSubmit =
    trimmedTitle.length > 0 &&
    selectedBoard.length > 0 &&
    trimmedStage.length > 0 &&
    stageOptions.length > 0 &&
    formState.language.trim().length > 0 &&
    formState.variant.trim().length > 0 &&
    formState.adaptationLabel.trim().length > 0 &&
    trimmedHTML.length > 0 &&
    !createMutation.isPending;
  const submit = () => {
    if (!canSubmit) {
      return;
    }

    createMutation.mutate({
      language: formState.language.trim() || "en",
      adaptation_label: formState.adaptationLabel.trim() || "Default",
      original_html: trimmedHTML,
      preheader: optionalString(formState.preheader),
      send_timing: optionalString(formState.sendTiming),
      sequence: selectedBoard,
      sort_order: selectedSortOrder,
      stage: trimmedStage,
      subject: optionalString(formState.subject),
      title: trimmedTitle,
      variant: formState.variant,
    });
  };
  const updateField = <Key extends keyof CreateEmailFormState>(
    key: Key,
    value: CreateEmailFormState[Key]
  ) => {
    setFormState((current) => ({ ...current, [key]: value }));
  };
  const loadHTMLFile = async (file: File | null) => {
    if (!file) {
      return;
    }

    updateField("originalHTML", await file.text());
    if (!formState.title.trim()) {
      updateField("title", file.name.replace(/\.html?$/i, ""));
    }
  };
  const handleBoardChange = (value: string | null) => {
    const nextBoard = value ?? "";
    const nextStageOptions =
      boards.find((board) => board.key === nextBoard)?.stages ?? [];
    const nextStage = nextStageOptions.includes(formState.stage)
      ? formState.stage
      : nextStageOptions[0] ?? "";

    setFormState((current) => ({
      ...current,
      sequence: nextBoard,
      stage: nextStage,
      eventGroupKey: newEventGroupValue,
    }));
    setSortOrderAuto(true);
  };
  const handleStageChange = (value: string | null) => {
    const nextStage = value ?? "";
    setFormState((current) => ({
      ...current,
      stage: nextStage,
      eventGroupKey: newEventGroupValue,
    }));
    setSortOrderAuto(true);
  };
  const handleEventGroupChange = (value: string | null) => {
    if (!value || value === newEventGroupValue) {
      setFormState((current) => ({
        ...current,
        eventGroupKey: newEventGroupValue,
      }));
      setSortOrderAuto(true);
      return;
    }

    const nextGroup = eventGroupOptions.find((option) => option.value === value);
    if (!nextGroup) {
      setFormState((current) => ({
        ...current,
        eventGroupKey: newEventGroupValue,
      }));
      setSortOrderAuto(true);
      return;
    }

    setFormState((current) => ({
      ...current,
      eventGroupKey: nextGroup.value,
      title: nextGroup.title,
      sendTiming: nextGroup.sendTiming ?? current.sendTiming,
      sortOrder: nextGroup.sortOrder,
      adaptationLabel: nextGroup.adaptationLabel,
    }));
    setSortOrderAuto(false);
  };

  const editorPanel = (
    <EmailCreateFormPanel
      adaptationOptions={adaptationOptions}
      boardOptions={boardOptions}
      boardsLoading={boardsQuery.isLoading}
      canSubmit={canSubmit}
      createError={createError}
      eventSelectOptions={eventSelectOptions}
      formState={formState}
      htmlInspection={htmlInspection}
      htmlInspectionError={htmlInspectionQuery.isError}
      htmlInspectionPending={htmlInspectionQuery.isPending}
      htmlInspectionWarnings={htmlInspectionWarnings}
      isCreating={createMutation.isPending}
      languageOptions={languageOptions}
      selectedBoard={selectedBoard}
      selectedEventGroup={selectedEventGroup}
      selectedSortOrder={selectedSortOrder}
      selectedStage={selectedStage}
      sortOrderAuto={sortOrderAuto}
      stageOptions={stageOptions}
      trimmedHTML={trimmedHTML}
      variantOptions={variantOptions}
      onBoardChange={handleBoardChange}
      onCreate={submit}
      onEventGroupChange={handleEventGroupChange}
      onHTMLFileLoad={(file) => void loadHTMLFile(file)}
      onSortOrderAutoChange={setSortOrderAuto}
      onStageChange={handleStageChange}
      onUpdateField={updateField}
    />
  );

  const previewPanel = (
    <EmailCreatePreview
      formState={formState}
      previewHTML={previewHTML}
      trimmedHTML={trimmedHTML}
    />
  );

  return (
    <div className={styles.page}>
      <EmailCreateHeader />

      <main className={styles.content}>
        <section className={styles.editorPanel}>{editorPanel}</section>
        <section className={styles.previewPanel}>{previewPanel}</section>
      </main>

      <Tabs className={styles.mobileTabs} defaultValue="details">
        <Tabs.List grow>
          <Tabs.Tab value="details">Details</Tabs.Tab>
          <Tabs.Tab value="preview">Preview</Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="details" pt="md">
          {editorPanel}
        </Tabs.Panel>
        <Tabs.Panel value="preview" pt="md">
          <section className={styles.previewPanel}>{previewPanel}</section>
        </Tabs.Panel>
      </Tabs>
    </div>
  );
}

export default EmailCreate;
