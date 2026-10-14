import {
  Group,
  Stack,
  Text,
} from "@mantine/core";
import { useMediaQuery } from "@mantine/hooks";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type SyntheticEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import { useBitrixVariables } from "../../bitrix-variables/api";
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

export type SelectionMenuState = ReviewTextSelection & {
  editableTarget: InlineEditableTarget | null;
  mode: "actions" | "comment" | "edit";
  x: number;
  y: number;
};

export type InlineEditInput = {
  field?: EditableField;
  key: string;
  label: string;
  type: "text" | "url" | "image" | "number";
  value: string | number;
};

export type InlineEditableTarget = {
  inputs: InlineEditInput[];
  label: string;
};

type MailPreviewProps = {
  activeCommentId?: string | null;
  canEditContent?: boolean;
  canEditHTML?: boolean;
  changedBlockTargets?: ReviewChangedBlockTarget[];
  commentTargets?: ReviewCommentTarget[];
  createCommentError?: boolean;
  email: EmailDetail;
  enableReviewSelectionComposer?: boolean;
  hoveredCommentId?: string | null;
  inlineEditError?: boolean;
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
  hoveredCommentId = null,
  inlineEditError = false,
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
  const overlayRootRef = useRef<HTMLElement | null>(null);
  const isApplyingInlineEditRef = useRef(isApplyingInlineEdit);
  const wasApplyingInlineEditRef = useRef(false);
  const wasCreatingCommentRef = useRef(false);
  const shouldShowCommentOverlay = !useMediaQuery("(max-width: 64em)");
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenuState | null>(
    null
  );
  const [draftComment, setDraftComment] = useState("");
  const [draftCommentSeverity, setDraftCommentSeverity] =
    useState<EmailCommentSeverity>("issue");
  const [draftInlineEdit, setDraftInlineEdit] = useState<
    Record<string, string | number>
  >({});
  const [frameLoadVersion, setFrameLoadVersion] = useState(0);
  const variables = useBitrixVariables().data;
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
      setFrameLoadVersion((version) => version + 1);

      if (enableReviewSelectionComposer) {
        installReviewSelectionMenu(event.currentTarget, email, (nextSelectionMenu) => {
          if (!isApplyingInlineEditRef.current) {
            setSelectionMenu(nextSelectionMenu);
          }
        });
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
        nextEditableFields[input.key] = {
          ...input.field,
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
    changedBlockTargets: shouldShowCommentOverlay ? changedBlockTargets : [],
    commentTargets: shouldShowCommentOverlay ? commentTargets : [],
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
        inlineEditError={inlineEditError}
        isApplyingInlineEdit={isApplyingInlineEdit}
        isCreatingComment={isCreatingComment}
        selectionMenu={selectionMenu}
        trimmedDraft={trimmedDraft}
        variables={variables}
        onApplyInlineEdit={handleApplyInlineEdit}
        onClose={closeSelectionComposer}
        onDraftCommentChange={setDraftComment}
        onDraftCommentSeverityChange={setDraftCommentSeverity}
        onDraftInlineEditChange={setDraftInlineEdit}
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
                <div className={styles.mailAvatar}>B</div>
                <Stack gap={0}>
                  <Text size="sm" fw={600}>
                    Bitrix24 Partners
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
                    Reply-To: partners@example.com
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
            {shouldShowCommentOverlay ? (
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
            ) : null}
          </div>
          {shouldShowCommentOverlay ? (
            <ReviewCommentOverlay
              activeCommentId={activeCommentId}
              badges={overlay.externalBadges}
              changedBlockRects={overlay.externalChangedBlockRects}
              hoveredCommentId={hoveredCommentId}
              onBadgeClick={onCommentBadgeClick}
              onBadgeHover={onCommentBadgeHover}
              rects={overlay.externalRects}
            />
          ) : null}
        </article>
      </div>
      {isScanning ? (
        <div className={styles.emailScanOverlay} aria-hidden="true" />
      ) : null}
    </div>
  );
}

export default MailPreview;
