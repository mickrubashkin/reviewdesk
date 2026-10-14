import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import { ApiError } from "../../../../shared/api";
import {
  archiveEmail,
  createCommentMessage,
  createEmailComment,
  duplicateEmail,
  resolveComment,
  restoreEmailVersion,
  updateEmailAreaApproval,
  updateEmailEditableFields,
  updateEmailPlanningFields,
  updateEmailReviewStatus,
} from "../../../emails/api";
import {
  formatEmailReviewStatus,
} from "../../../emails/reviewStatus";
import type {
  DuplicateEmailPayload,
  EmailCommentSeverity,
  EmailReviewStatus,
  UpdateEditableFieldsPayload,
  UpdateEmailPlanningFieldsPayload,
} from "../../../emails/types";
import type { ReviewTextSelection } from "../../../emails/MailPreview.types";
import {
  applyPlanningFieldsToCaches,
  applyReviewStatusToCaches,
} from "../EmailReview.helpers";

export function useEmailReviewMutations({
  approvalBlockedMessage,
  emailId,
  sequence,
}: {
  approvalBlockedMessage: string;
  emailId: string;
  sequence?: string;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const duplicateEmailMutation = useMutation({
    mutationFn: ({
      sourceEmailId,
      payload,
    }: {
      sourceEmailId: string;
      payload: DuplicateEmailPayload;
    }) => duplicateEmail(sourceEmailId, payload),
    onSuccess: (createdEmail) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
      queryClient.setQueryData(["emails", createdEmail.id, "review"], createdEmail);
      navigate(`/emails/${encodeURIComponent(createdEmail.id)}/review`);
    },
  });

  const archiveEmailMutation = useMutation({
    mutationFn: archiveEmail,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
      notifications.show({
        color: "green",
        message: "Email was removed from the active board.",
        title: "Email archived",
      });
      navigate("/");
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Archive failed",
      });
    },
  });

  const reviewStatusMutation = useMutation({
    mutationFn: ({
      nextStatus,
      targetEmailId,
    }: {
      nextStatus: EmailReviewStatus;
      targetEmailId: string;
    }) =>
      updateEmailReviewStatus(targetEmailId, {
        review_status: nextStatus,
      }),
    onSuccess: (response, variables) => {
      applyReviewStatusToCaches(
        queryClient,
        variables.targetEmailId,
        response.review_status,
        sequence
      );
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", variables.targetEmailId],
      });
      notifications.show({
        color: "green",
        message: `Review status changed to ${formatEmailReviewStatus(response.review_status)}.`,
        title: "Status updated",
      });
    },
    onError: (error) => {
      const conflictMessage =
        error instanceof ApiError && error.status === 409
          ? error.message.trim() || approvalBlockedMessage
          : null;
      notifications.show({
        color: "red",
        message: conflictMessage ?? "Try again or check that you have admin access.",
        title: conflictMessage ? "Approval blocked" : "Status update failed",
      });
    },
  });

  const planningFieldsMutation = useMutation({
    mutationFn: ({
      payload,
      targetEmailId,
    }: {
      payload: UpdateEmailPlanningFieldsPayload;
      targetEmailId: string;
    }) => updateEmailPlanningFields(targetEmailId, payload),
    onSuccess: (response, variables) => {
      applyPlanningFieldsToCaches(
        queryClient,
        variables.targetEmailId,
        response,
        sequence
      );
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", variables.targetEmailId],
      });
      notifications.show({
        color: "green",
        message: "Planning fields updated.",
        title: "Planning updated",
      });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Planning update failed",
      });
    },
  });

  const areaApprovalMutation = useMutation({
    mutationFn: ({
      area,
      decisionNote,
      status,
    }: {
      area: string;
      decisionNote: string | null;
      status: "approved" | "changes_requested";
    }) =>
      updateEmailAreaApproval(emailId, area, {
        decision_note: decisionNote,
        status,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["email-area-approvals", emailId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
      notifications.show({
        color: "green",
        message: "Approval area updated.",
        title: "Approval updated",
      });
    },
    onError: () => {
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Approval update failed",
      });
    },
  });

  const createCommentMutation = useMutation({
    mutationFn: ({
      body,
      severity,
      selection,
    }: {
      body: string;
      severity: EmailCommentSeverity;
      selection: ReviewTextSelection;
    }) =>
      createEmailComment(emailId, {
        review_block: selection.reviewBlock,
        selected_text: selection.selectedText,
        start_offset: selection.startOffset,
        end_offset: selection.endOffset,
        body,
        severity,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["email-comments", emailId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
    },
  });

  const resolveCommentMutation = useMutation({
    mutationFn: (commentId: string) => resolveComment(commentId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["email-comments", emailId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
    },
  });

  const createReplyMutation = useMutation({
    mutationFn: ({
      body,
      commentId,
    }: {
      body: string;
      commentId: string;
    }) => createCommentMessage(commentId, { body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["email-comments", emailId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
    },
  });

  const restoreVersionMutation = useMutation({
    mutationFn: (versionId: string) => restoreEmailVersion(emailId, versionId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails", emailId, "review"] });
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({
        queryKey: ["emails", emailId, "rendered"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["emails", emailId, "versions"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
      notifications.show({
        color: "green",
        message: "Version was restored.",
        title: "Version restored",
      });
    },
  });

  const saveEditableFieldsMutation = useMutation({
    mutationFn: (payload: UpdateEditableFieldsPayload) =>
      updateEmailEditableFields(emailId, payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails", emailId, "review"] });
      void queryClient.invalidateQueries({
        queryKey: ["emails", emailId, "rendered"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["emails", emailId, "versions"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-area-approvals", emailId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["email-activity", emailId],
      });
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
    },
  });

  return {
    duplicateEmailMutation,
    archiveEmailMutation,
    reviewStatusMutation,
    planningFieldsMutation,
    areaApprovalMutation,
    createCommentMutation,
    resolveCommentMutation,
    createReplyMutation,
    restoreVersionMutation,
    saveEditableFieldsMutation,
  };
}
