import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Modal,
  Paper,
  Select,
  Stack,
  Text,
  Textarea,
  TextInput,
  Tooltip,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useSearchParams } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import { fetchBoards } from "../../boards/api";
import { canManageBoard, managesAnyBoard } from "../../emails/permissions";
import type { AuthUser } from "../../emails/types";
import {
  createBoardSharedText,
  deleteBoardSharedText,
  sharedTextsQueryKey,
  updateSharedText,
  useSharedTexts,
  type SharedText,
  type UpdateSharedTextResponse,
} from "../api";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const keyLabels: Record<string, string> = {
  footer_logo_src: "Footer logo",
  footer_logo_alt: "Footer logo, alt text",
  footer_copyright: "Copyright",
  footer_legal_text: "Legal text",
  privacy_policy_link_text: "Privacy policy, link text",
  privacy_policy_link_url: "Privacy policy, link",
  signature: "Signature",
  login_here_text: "Login block, “Login here”",
  login_label: "Login block, login label",
  password_label: "Login block, password label",
  password_security_note_text: "Login block, password note",
  password_security_note_url: "Login block, password note link",
  partner_login_link_text: "Login block, portal link text",
  partner_login_link_url: "Login block, portal link",
};

const languageOrder = ["en", "de", "es", "pl", "br"];

function languageRank(language: string) {
  const index = languageOrder.indexOf(language);
  return index === -1 ? languageOrder.length : index;
}

function keyLabel(key: string) {
  return keyLabels[key] ?? key.replaceAll("_", " ");
}

// What the editor is open on: a text to edit, or a board's new version of a
// common text.
type EditorTarget = { text: SharedText; create: boolean };

// One language of one key: the text for all boards and the selected board's
// own version, if any.
type SharedTextRowData = { common: SharedText; own?: SharedText };

export function SharedTexts({ currentUser }: { currentUser: AuthUser }) {
  const isSuperAdmin = currentUser.role === "super_admin";
  const canOpen = isSuperAdmin || managesAnyBoard(currentUser);
  const textsQuery = useSharedTexts(canOpen);
  const boardsQuery = useQuery({
    queryKey: ["boards"],
    queryFn: fetchBoards,
    enabled: canOpen,
  });
  const [searchParams, setSearchParams] = useSearchParams();
  const [editorTarget, setEditorTarget] = useState<EditorTarget | null>(null);
  const [removing, setRemoving] = useState<SharedText | null>(null);

  // Super admins edit the texts for all boards; a board's own versions are
  // for the people who manage the board.
  const boardOptions = useMemo(
    () => [
      ...(isSuperAdmin ? [{ value: "", label: "All boards" }] : []),
      ...(boardsQuery.data ?? [])
        .filter((board) => canManageBoard(currentUser, board.key))
        .map((board) => ({ value: board.key, label: board.name })),
    ],
    [boardsQuery.data, currentUser, isSuperAdmin]
  );
  const requestedBoard = searchParams.get("board") ?? "";
  const board = boardOptions.some((option) => option.value === requestedBoard)
    ? requestedBoard
    : (boardOptions[0]?.value ?? "");
  const boardNames = useMemo(
    () => new Map((boardsQuery.data ?? []).map((item) => [item.key, item.name])),
    [boardsQuery.data]
  );

  const groups = useMemo(() => {
    const texts = textsQuery.data ?? [];
    const byKey = new Map<string, SharedTextRowData[]>();
    for (const common of texts.filter((text) => text.board === "")) {
      const own = board
        ? texts.find(
            (text) =>
              text.board === board && text.key === common.key && text.language === common.language
          )
        : undefined;
      byKey.set(common.key, [...(byKey.get(common.key) ?? []), { common, own }]);
    }
    const keys = [...byKey.keys()].sort((a, b) => {
      const order = Object.keys(keyLabels);
      const rank = (key: string) => (order.includes(key) ? order.indexOf(key) : order.length);
      return rank(a) - rank(b) || a.localeCompare(b);
    });
    return keys.map((key) => ({
      key,
      rows: (byKey.get(key) ?? []).sort(
        (a, b) => languageRank(a.common.language) - languageRank(b.common.language)
      ),
    }));
  }, [board, textsQuery.data]);

  if (!canOpen) {
    return <Navigate replace to="/" />;
  }

  // Boards with their own version of a common text, by name.
  const ownVersionBoards = (common: SharedText) =>
    (textsQuery.data ?? [])
      .filter(
        (text) => text.board !== "" && text.key === common.key && text.language === common.language
      )
      .map((text) => boardNames.get(text.board) ?? text.board);
  // The EN text a translation is checked against: the board's own EN version
  // if there is one.
  const englishFor = (text: SharedText) => {
    if (text.language === "en") {
      return undefined;
    }
    const texts = textsQuery.data ?? [];
    const isEnglish = (item: SharedText) => item.key === text.key && item.language === "en";
    return (
      (board ? texts.find((item) => isEnglish(item) && item.board === board) : undefined) ??
      texts.find((item) => isEnglish(item) && item.board === "")
    );
  };
  const boardName = boardNames.get(board) ?? board;

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Footer, privacy link, logo and signature: one text per language for all emails that use it. A board can have its own version, for example a regional footer."
        title="Shared texts"
      />
      <Stack maw={960} mx="auto" p="md">
        {boardOptions.length > 1 ? (
          <Select
            allowDeselect={false}
            data={boardOptions}
            label="Board"
            maw={320}
            value={board}
            onChange={(value) => setSearchParams(value ? { board: value } : {}, { replace: true })}
          />
        ) : null}
        <Text c="dimmed" size="sm">
          {board
            ? `Emails on ${boardName} use the common text unless the board has its own version. The board’s own version changes only this board’s emails.`
            : "Saving a text writes a new version of every email that uses it, except on boards with their own version."}{" "}
          Live emails then need a re-upload to Bitrix24 (they show up in “To upload”). An email can
          keep its own text instead: in the email, click the field and choose “Use own text”.
        </Text>
        {textsQuery.isLoading || boardsQuery.isLoading ? (
          <Loader />
        ) : textsQuery.isError ? (
          <Alert color="red">Failed to load shared texts.</Alert>
        ) : groups.length === 0 ? (
          <Alert color="gray">
            No shared texts yet. They are created from the emails’ current footers with{" "}
            <code>go run ./cmd/sharedtexts</code>.
          </Alert>
        ) : (
          groups.map((group) => (
            <Paper key={group.key} p="sm" withBorder>
              <Group gap="xs" mb="xs">
                <Text fw={700} size="sm">
                  {keyLabel(group.key)}
                </Text>
                <Text c="dimmed" ff="monospace" size="xs">
                  {group.key}
                </Text>
              </Group>
              <Stack gap={6}>
                {group.rows.map(({ common, own }) => (
                  <SharedTextRow
                    key={common.language}
                    boardSelected={board !== ""}
                    ownVersionBoards={board ? [] : ownVersionBoards(common)}
                    text={own ?? common}
                    onCreate={
                      board && !own
                        ? () =>
                            setEditorTarget({
                              text: { ...common, board },
                              create: true,
                            })
                        : undefined
                    }
                    onEdit={
                      board && !own
                        ? undefined
                        : () =>
                            setEditorTarget({
                              text: own ?? common,
                              create: false,
                            })
                    }
                    onRemove={own ? () => setRemoving(own) : undefined}
                  />
                ))}
              </Stack>
            </Paper>
          ))
        )}
      </Stack>
      {editorTarget ? (
        <SharedTextEditor
          boardName={
            editorTarget.text.board
              ? (boardNames.get(editorTarget.text.board) ?? editorTarget.text.board)
              : ""
          }
          create={editorTarget.create}
          english={englishFor(editorTarget.text)}
          text={editorTarget.text}
          onClose={() => setEditorTarget(null)}
        />
      ) : null}
      {removing ? (
        <RemoveBoardVersionModal
          boardName={boardNames.get(removing.board) ?? removing.board}
          common={(textsQuery.data ?? []).find(
            (text) =>
              text.board === "" && text.key === removing.key && text.language === removing.language
          )}
          text={removing}
          onClose={() => setRemoving(null)}
        />
      ) : null}
    </div>
  );
}

function SharedTextRow({
  boardSelected,
  ownVersionBoards,
  text,
  onCreate,
  onEdit,
  onRemove,
}: {
  boardSelected: boolean;
  ownVersionBoards: string[];
  text: SharedText;
  onCreate?: () => void;
  onEdit?: () => void;
  onRemove?: () => void;
}) {
  const inherited = boardSelected && text.board === "";
  return (
    <Group align="flex-start" gap="sm" wrap="nowrap">
      <Badge miw={38} variant="light">
        {text.language.toUpperCase()}
      </Badge>
      <Stack flex={1} gap={2} miw={0}>
        <Text
          c={inherited ? "dimmed" : undefined}
          lineClamp={2}
          size="sm"
          style={{ overflowWrap: "anywhere", whiteSpace: "pre-line" }}
        >
          {text.value}
        </Text>
        <Group gap="xs">
          {inherited ? (
            <Text c="dimmed" size="xs">
              Common text
            </Text>
          ) : (
            <>
              {boardSelected ? (
                <Badge color="grape" size="xs" variant="light">
                  This board
                </Badge>
              ) : null}
              <Text c="dimmed" size="xs">
                {text.linked_emails} {text.linked_emails === 1 ? "email" : "emails"}
                {text.linked_live > 0 ? ` · ${text.linked_live} live` : ""}
              </Text>
              {text.own_emails > 0 ? (
                <Tooltip
                  label="Emails with this field that keep a different text of their own"
                  multiline
                  w={240}
                >
                  <Text c="dimmed" size="xs">
                    · {text.own_emails} with own text
                  </Text>
                </Tooltip>
              ) : null}
            </>
          )}
          {ownVersionBoards.length > 0 ? (
            <Tooltip label={ownVersionBoards.join(", ")} multiline w={240}>
              <Text c="dimmed" size="xs">
                · own version on {ownVersionBoards.length}{" "}
                {ownVersionBoards.length === 1 ? "board" : "boards"}
              </Text>
            </Tooltip>
          ) : null}
          {text.en_changed && !inherited ? (
            <Badge color="orange" size="xs" variant="light">
              EN changed
            </Badge>
          ) : null}
        </Group>
      </Stack>
      <Group gap={4} wrap="nowrap">
        {onRemove ? (
          <Button color="gray" size="xs" variant="subtle" onClick={onRemove}>
            Use common text
          </Button>
        ) : null}
        {onCreate ? (
          <Button size="xs" variant="subtle" onClick={onCreate}>
            Own version
          </Button>
        ) : null}
        {onEdit ? (
          <Button size="xs" variant="subtle" onClick={onEdit}>
            Edit
          </Button>
        ) : null}
      </Group>
    </Group>
  );
}

function savedMessage(result: UpdateSharedTextResponse) {
  return result.updated_emails > 0
    ? `${result.updated_emails} ${result.updated_emails === 1 ? "email" : "emails"} got a new version` +
        (result.live_emails > 0 ? `, ${result.live_emails} live: re-upload them to Bitrix24.` : ".")
    : "No email changed.";
}

function RemoveBoardVersionModal({
  boardName,
  common,
  text,
  onClose,
}: {
  boardName: string;
  common?: SharedText;
  text: SharedText;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const removeMutation = useMutation({
    mutationFn: () => deleteBoardSharedText(text),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: sharedTextsQueryKey });
      await queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        title: "Board version removed",
        message: savedMessage(result),
      });
      onClose();
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        message: error.message,
        title: "Remove failed",
      }),
  });

  return (
    <Modal
      opened
      title={`${keyLabel(text.key)} · ${text.language.toUpperCase()} · ${boardName}`}
      onClose={onClose}
    >
      <Stack gap="sm">
        <Text size="sm">
          The board’s {text.linked_emails} {text.linked_emails === 1 ? "email" : "emails"} go back
          to the common text:
        </Text>
        <Text c="dimmed" size="sm" style={{ overflowWrap: "anywhere", whiteSpace: "pre-line" }}>
          {common?.value}
        </Text>
        {text.linked_live > 0 ? (
          <Alert color="orange" variant="light">
            {text.linked_live} of them live: each needs a re-upload to Bitrix24.
          </Alert>
        ) : null}
        <Group justify="flex-end" gap="xs">
          <Button disabled={removeMutation.isPending} variant="subtle" onClick={onClose}>
            Cancel
          </Button>
          <Button
            color="red"
            loading={removeMutation.isPending}
            onClick={() => removeMutation.mutate()}
          >
            Use common text
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

// The editor of a text, or with create of a board's new version of the
// common text, which starts from the common value.
function SharedTextEditor({
  boardName,
  create,
  english,
  text,
  onClose,
}: {
  boardName: string;
  create: boolean;
  english?: SharedText;
  text: SharedText;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState(text.value);
  const changed = value !== text.value;

  const saveMutation = useMutation({
    mutationFn: (nextValue: string) =>
      create
        ? createBoardSharedText(text, text.board, nextValue)
        : updateSharedText(text, nextValue),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: sharedTextsQueryKey });
      await queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        title: create
          ? "Board version created"
          : result.updated_emails > 0
            ? "Shared text saved"
            : "Marked as up to date",
        message: savedMessage(result),
      });
      onClose();
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        message: error.message,
        title: "Save failed",
      }),
  });

  const multiline = text.type === "text" && (text.value.length > 60 || text.value.includes("\n"));

  return (
    <Modal
      opened
      size="lg"
      title={`${keyLabel(text.key)} · ${text.language.toUpperCase()}${boardName ? ` · ${boardName}` : ""}`}
      onClose={onClose}
    >
      <Stack gap="sm">
        {english ? (
          <Stack gap={2}>
            <Group gap="xs">
              <Text c="dimmed" size="xs">
                EN
              </Text>
              {text.en_changed && !create ? (
                <Badge color="orange" size="xs" variant="light">
                  changed since this translation was saved
                </Badge>
              ) : null}
            </Group>
            <Text size="sm" style={{ overflowWrap: "anywhere", whiteSpace: "pre-line" }}>
              {english.value}
            </Text>
          </Stack>
        ) : null}
        {multiline ? (
          <Textarea
            autosize
            data-autofocus
            label="Text"
            minRows={3}
            value={value}
            onChange={(event) => setValue(event.currentTarget.value)}
          />
        ) : (
          <TextInput
            data-autofocus
            label={text.type === "text" ? "Text" : "Link"}
            value={value}
            onChange={(event) => setValue(event.currentTarget.value)}
          />
        )}
        {create ? (
          <Alert color="blue" variant="light">
            The emails on {boardName} that use the common text switch to this version and get a new
            version each; live ones need a re-upload to Bitrix24. Other boards keep the common text.
          </Alert>
        ) : changed ? (
          <Alert color={text.linked_live > 0 ? "orange" : "blue"} variant="light">
            Saving writes a new version of {text.linked_emails}{" "}
            {text.linked_emails === 1 ? "email" : "emails"}
            {text.linked_live > 0
              ? `, ${text.linked_live} of them live: each needs a re-upload to Bitrix24.`
              : "."}
            {text.language === "en" ? " Translations of this text will show “EN changed”." : ""}
          </Alert>
        ) : null}
        <Group justify="space-between">
          <Text c="dimmed" size="xs">
            {create ? "Starts from the common text" : `v${text.version}`}
            {text.updated_by && !create ? ` · ${text.updated_by}` : ""}
          </Text>
          <Group gap="xs">
            <Button disabled={saveMutation.isPending} variant="subtle" onClick={onClose}>
              Cancel
            </Button>
            {!changed && text.en_changed && !create ? (
              <Button
                loading={saveMutation.isPending}
                variant="light"
                onClick={() => saveMutation.mutate(text.value)}
              >
                Still correct
              </Button>
            ) : null}
            <Button
              disabled={(!changed && !create) || value.trim() === ""}
              loading={saveMutation.isPending}
              onClick={() => saveMutation.mutate(value)}
            >
              Save
            </Button>
          </Group>
        </Group>
      </Stack>
    </Modal>
  );
}
