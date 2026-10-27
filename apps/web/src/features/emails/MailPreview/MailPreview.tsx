import {
  Group,
  Stack,
  Text,
} from "@mantine/core";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type SyntheticEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import { useBitrixVariables } from "../../bitrix-variables/api";
import { sharedTextsForEmail, useSharedTexts } from "../../shared-texts/api";
import { ReviewCommentOverlay } from "../ReviewCommentOverlay";
import type {
  EditableField,
  EditableFields,
  EmailCommentSeverity,
  EmailDetail,
} from "../types";
import type {
  InlineEditUpdate,
  PreviewViewport,
  ReviewTextSelection,
} from "../MailPreview.types";
import type {
  ReviewChangedBlockTarget,
  ReviewCommentTarget,
} from "../reviewOverlayTypes";
import { useReviewOverlayRects } from "../useReviewOverlayRects";
import {
  getHeaderEditableTarget,
  installReviewSelectionMenu,
} from "./MailPreview.helpers";
import { markBitrixVariables } from "./bitrixVariables";
import { SelectionComposer } from "./SelectionComposer";
import styles from "../EmailPreviewDrawer.module.css";
import { previewSender } from "../../../shared/brand";

export type SelectionMenuState = ReviewTextSelection & {
  editableTarget: InlineEditableTarget | null;
  mode: "actions" | "comment" | "edit";
  placement?: "top" | "bottom";
  x: number;
  y: number;
};

export type InlineEditInput = {
  field?: EditableField;
  key: string;
  label: string;
  type: "text" | "url" | "image" | "number";
  value: string | number;
  // A text field on a block element (td, th, div) can be Markdown; the
  // server makes it Markdown on its first edit.
  formattable?: boolean;
};

export type InlineEditableTarget = {
  inputs: InlineEditInput[];
  label: string;
};

const noFieldLimits: Record<string, number> = {};

type MailPreviewProps = {
  activeCommentId?: string | null;
  canEditContent?: boolean;
  canEditHTML?: boolean;
  changedBlockTargets?: ReviewChangedBlockTarget[];
  commentTargets?: ReviewCommentTarget[];
  createCommentError?: boolean;
  email: EmailDetail;
  enableReviewSelectionComposer?: boolean;
  // How long a field may be, by key; the editor counts the characters.
  fieldLimits?: Record<string, number>;
  hoveredCommentId?: string | null;
  inlineEditError?: boolean;
  // What the API said when it refused the edit, e.g. an empty required field.
  inlineEditErrorMessage?: string;
  isApplyingInlineEdit?: boolean;
  isCreatingComment?: boolean;
  isScanning: boolean;
  onApplyInlineEdit?: (update: InlineEditUpdate) => void;
  onCommentBadgeClick?: (commentIds: string[]) => void;
  onCommentBadgeHover?: (commentIds: string[] | null) => void;
  onCreateReviewComment?: (
    selection: ReviewTextSelection,
    body: string,
    severity: EmailCommentSeverity
  ) => void;
  onEditSourceHTML?: () => void;
  onInlineEditPreviewReady?: () => void;
  viewport: PreviewViewport;
};

export function MailPreview({
  activeCommentId = null,
  canEditContent = false,
  canEditHTML = false,
  changedBlockTargets = [],
  commentTargets = [],
  createCommentError = false,
  email,
  enableReviewSelectionComposer = false,
  fieldLimits = noFieldLimits,
  hoveredCommentId = null,
  inlineEditError = false,
  inlineEditErrorMessage,
  isApplyingInlineEdit = false,
  isCreatingComment = false,
  isScanning,
  onApplyInlineEdit,
  onCommentBadgeClick,
  onCommentBadgeHover,
  onCreateReviewComment,
  onEditSourceHTML,
  onInlineEditPreviewReady,
  viewport,
}: MailPreviewProps) {
  const frameRef = useRef<HTMLIFrameElement | null>(null);
  const frameOverlayLayerRef = useRef<HTMLDivElement | null>(null);
  const frameInputLayerRef = useRef<HTMLDivElement | null>(null);
  const overlayRootRef = useRef<HTMLElement | null>(null);
  const isApplyingInlineEditRef = useRef(isApplyingInlineEdit);
  const wasApplyingInlineEditRef = useRef(false);
  const wasCreatingCommentRef = useRef(false);
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenuState | null>(
    null
  );
  const [draftComment, setDraftComment] = useState("");
  const [draftCommentSeverity, setDraftCommentSeverity] =
    useState<EmailCommentSeverity>("issue");
  const [draftInlineEdit, setDraftInlineEdit] = useState<
    Record<string, string | number>
  >({});
  // Fields switched to or from their shared text in the open editor.
  const [draftShared, setDraftShared] = useState<Record<string, boolean>>({});
  const [frameLoadVersion, setFrameLoadVersion] = useState(0);
  const variables = useBitrixVariables().data;
  const sharedTextsQuery = useSharedTexts(canEditContent);
  const sharedTexts = useMemo(
    () => sharedTextsForEmail(sharedTextsQuery.data, email.language, email.sequence),
    [email.language, email.sequence, sharedTextsQuery.data]
  );
  useEffect(() => {
    if (variables) {
      markBitrixVariables(frameRef.current?.contentDocument, variables);
    }
  }, [frameLoadVersion, variables]);
  const handleFrameLoad = useCallback(
    (event: SyntheticEvent<HTMLIFrameElement>) => {
      frameRef.current = event.currentTarget;
      setSelectionMenu(null);
      setDraftComment("");
      setDraftInlineEdit({});
      setDraftShared({});
      setFrameLoadVersion((version) => version + 1);

      if (enableReviewSelectionComposer) {
        installReviewSelectionMenu(
          event.currentTarget,
          email,
          (nextSelectionMenu) => {
            if (!isApplyingInlineEditRef.current) {
              setSelectionMenu(nextSelectionMenu);
            }
          },
          frameInputLayerRef.current
        );
      }

      if (isApplyingInlineEditRef.current) {
        onInlineEditPreviewReady?.();
      }
    },
    [email, enableReviewSelectionComposer, onInlineEditPreviewReady]
  );
  const closeSelectionComposer = () => {
    setSelectionMenu(null);
    setDraftComment("");
    setDraftCommentSeverity("issue");
    setDraftInlineEdit({});
    setDraftShared({});
  };
  const openCommentComposer = () => {
    setSelectionMenu((current) =>
      current ? { ...current, mode: "comment" } : current
    );
    setDraftComment("");
    setDraftCommentSeverity("issue");
  };
  const openInlineEditor = () => {
    if (isApplyingInlineEdit) {
      return;
    }

    setSelectionMenu((current) => {
      if (!current?.editableTarget) {
        return current;
      }

      setDraftInlineEdit(
        Object.fromEntries(
          current.editableTarget.inputs.map((input) => [input.key, input.value])
        )
      );
      setDraftShared({});
      return { ...current, mode: "edit" };
    });
  };
  const handleSubmitComment = () => {
    if (!selectionMenu) {
      return;
    }

    const trimmedDraft = draftComment.trim();
    if (!trimmedDraft) {
      return;
    }

    onCreateReviewComment?.(
      {
        reviewBlock: selectionMenu.reviewBlock,
        selectedText: selectionMenu.selectedText,
        startOffset: selectionMenu.startOffset,
        endOffset: selectionMenu.endOffset,
      },
      trimmedDraft,
      draftCommentSeverity
    );
  };
  const handleApplyInlineEdit = () => {
    const target = selectionMenu?.editableTarget;
    if (!target) {
      return;
    }

    const nextEditableFields: EditableFields = {};
    let nextSubject: string | undefined;
    let nextPreheader: string | undefined;

    for (const input of target.inputs) {
      const value = draftInlineEdit[input.key] ?? input.value;
      if (input.key === "subject") {
        nextSubject = String(value);
        continue;
      }
      if (input.key === "preheader") {
        nextPreheader = String(value);
        continue;
      }
      if (input.field) {
        // A shared field gets its value from the shared text on the server.
        const shared = draftShared[input.key] ?? Boolean(input.field.shared);
        nextEditableFields[input.key] = {
          ...input.field,
          shared,
          value: input.type === "number" ? Number(value) || 0 : String(value),
        };
      }
    }

    onApplyInlineEdit?.({
      ...(Object.keys(nextEditableFields).length > 0
        ? { editableFields: nextEditableFields }
        : {}),
      ...(nextSubject !== undefined ? { subject: nextSubject } : {}),
      ...(nextPreheader !== undefined ? { preheader: nextPreheader } : {}),
    });
  };
  const trimmedDraft = draftComment.trim();
  const openHeaderReviewComposer = (
    event: ReactMouseEvent<HTMLElement> | ReactKeyboardEvent<HTMLElement>,
    reviewBlock: string,
    selectedText: string
  ) => {
    if (isApplyingInlineEdit) {
      return;
    }

    const trimmedText = selectedText.trim();
    if (!trimmedText) {
      return;
    }

    const rect = event.currentTarget.getBoundingClientRect();
    setSelectionMenu({
      reviewBlock,
      editableTarget: getHeaderEditableTarget(reviewBlock, selectedText),
      mode: "actions",
      selectedText: trimmedText,
      startOffset: 0,
      endOffset: trimmedText.length,
      x: rect.left + rect.width / 2,
      y: rect.top,
    });
  };
  const handleHeaderReviewKeyDown = (
    event: ReactKeyboardEvent<HTMLElement>,
    reviewBlock: string,
    selectedText: string
  ) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openHeaderReviewComposer(event, reviewBlock, selectedText);
    }
  };
  const overlay = useReviewOverlayRects({
    activeCommentId,
    changedBlockTargets,
    commentTargets,
    frameOverlayLayerRef,
    frameLoadVersion,
    frameRef,
    overlayRootRef,
    viewport,
  });

  useEffect(() => {
    isApplyingInlineEditRef.current = isApplyingInlineEdit;
  }, [isApplyingInlineEdit]);

  useEffect(() => {
    if (wasCreatingCommentRef.current && !isCreatingComment && !createCommentError) {
      closeSelectionComposer();
    }

    wasCreatingCommentRef.current = isCreatingComment;
  }, [createCommentError, isCreatingComment]);

  useEffect(() => {
    if (
      wasApplyingInlineEditRef.current &&
      !isApplyingInlineEdit &&
      !inlineEditError
    ) {
      closeSelectionComposer();
    }

    wasApplyingInlineEditRef.current = isApplyingInlineEdit;
  }, [inlineEditError, isApplyingInlineEdit]);

  return (
    <div className={styles.emailPreviewWrap} data-viewport={viewport}>
      <SelectionComposer
        canEditContent={canEditContent}
        canEditHTML={canEditHTML}
        createCommentError={createCommentError}
        draftComment={draftComment}
        draftCommentSeverity={draftCommentSeverity}
        draftInlineEdit={draftInlineEdit}
        draftShared={draftShared}
        fieldLimits={fieldLimits}
        inlineEditError={inlineEditError}
        inlineEditErrorMessage={inlineEditErrorMessage}
        isApplyingInlineEdit={isApplyingInlineEdit}
        isCreatingComment={isCreatingComment}
        language={email.language}
        selectionMenu={selectionMenu}
        sharedTexts={sharedTexts}
        trimmedDraft={trimmedDraft}
        variables={variables}
        onApplyInlineEdit={handleApplyInlineEdit}
        onClose={closeSelectionComposer}
        onDraftCommentChange={setDraftComment}
        onDraftCommentSeverityChange={setDraftCommentSeverity}
        onDraftInlineEditChange={setDraftInlineEdit}
        onDraftSharedChange={setDraftShared}
        onEditSourceHTML={onEditSourceHTML}
        onOpenCommentComposer={openCommentComposer}
        onOpenInlineEditor={openInlineEditor}
        onSubmitComment={handleSubmitComment}
      />

      {isApplyingInlineEdit ? (
        <div className={styles.inlineEditRefreshNotice}>
          <Text fw={600} size="xs">
            Saving changes and refreshing preview...
          </Text>
        </div>
      ) : null}

      <div className={styles.mailClient}>
        <article className={styles.mailReadPane} ref={overlayRootRef}>
          <header className={styles.mailHeader}>
            <Group className={styles.mailMetaRow} justify="space-between" gap="sm">
              <Group gap="sm" wrap="nowrap">
                <div className={styles.mailAvatar}>{previewSender.initial}</div>
                <Stack gap={0}>
                  <Text size="sm" fw={600}>
                    {previewSender.name}
                  </Text>
                  <Text
                    className={styles.headerReviewBlock}
                    data-review-block="subject"
                    role="button"
                    size="sm"
                    tabIndex={0}
                    onClick={(event) =>
                      openHeaderReviewComposer(
                        event,
                        "subject",
                        email.subject ?? email.title
                      )
                    }
                    onKeyDown={(event) =>
                      handleHeaderReviewKeyDown(
                        event,
                        "subject",
                        email.subject ?? email.title
                      )
                    }
                  >
                    {email.subject ?? email.title}
                  </Text>
                  {email.preheader ? (
                    <Text
                      className={styles.headerReviewBlock}
                      c="dimmed"
                      data-review-block="preheader"
                      role="button"
                      size="xs"
                      tabIndex={0}
                      onClick={(event) =>
                        openHeaderReviewComposer(
                          event,
                          "preheader",
                          email.preheader ?? ""
                        )
                      }
                      onKeyDown={(event) =>
                        handleHeaderReviewKeyDown(
                          event,
                          "preheader",
                          email.preheader ?? ""
                        )
                      }
                    >
                      {email.preheader}
                    </Text>
                  ) : null}
                  <Text size="xs" c="dimmed">
                    Reply-To: {previewSender.replyTo}
                  </Text>
                </Stack>
              </Group>
            </Group>
          </header>

          <div className={styles.emailPreviewFrameWrap}>
            <iframe
              className={styles.emailPreviewFrame}
              onLoad={handleFrameLoad}
              ref={frameRef}
              title={email.title}
              // allow-same-origin lets the overlay, comment selection, and inline
              // editing read the frame document. Never add allow-scripts here: with
              // both flags the email HTML could remove its own sandbox.
              sandbox="allow-same-origin"
              srcDoc={email.review_html || email.original_html}
            />
            <div className={styles.frameInputLayer} hidden ref={frameInputLayerRef} />
            <ReviewCommentOverlay
              activeCommentId={activeCommentId}
              badges={overlay.frameBadges}
              changedBlockRects={overlay.frameChangedBlockRects}
              hoveredCommentId={hoveredCommentId}
              layerRef={frameOverlayLayerRef}
              onBadgeClick={onCommentBadgeClick}
              onBadgeHover={onCommentBadgeHover}
              rects={overlay.frameRects}
            />
          </div>
          <ReviewCommentOverlay
            activeCommentId={activeCommentId}
            badges={overlay.externalBadges}
            changedBlockRects={overlay.externalChangedBlockRects}
            hoveredCommentId={hoveredCommentId}
            onBadgeClick={onCommentBadgeClick}
            onBadgeHover={onCommentBadgeHover}
            rects={overlay.externalRects}
          />
        </article>
      </div>
      {isScanning ? (
        <div className={styles.emailScanOverlay} aria-hidden="true" />
      ) : null}
    </div>
  );
}

export default MailPreview;
