import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import { ApiError } from "../../../../shared/api";
import {
  archiveEmail,
  duplicateEmail,
  restoreEmail,
  restoreEmailVersion,
  updateEmailAreaApproval,
  updateEmailEditableFields,
  updateEmailLayout,
  updateEmailPlanningFields,
  updateEmailReviewStatus,
} from "../../../emails/api";
import {
  formatEmailReviewStatus,
} from "../../../emails/reviewStatus";
import type {
  DuplicateEmailPayload,
  EmailReviewStatus,
  LayoutItem,
  UpdateEditableFieldsPayload,
  UpdateEmailPlanningFieldsPayload,
} from "../../../emails/types";
import {
  applyPlanningFieldsToCaches,
  applyReviewStatusToCaches,
} from "../EmailReview.helpers";
import { useCommentMutations } from "../../../comments/useCommentMutations";
import { sharedTextsQueryKey } from "../../../shared-texts/api";

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
        message: "It moved to the board's Archive, where it can be restored.",
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

  const restoreEmailMutation = useMutation({
    mutationFn: restoreEmail,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", emailId] });
      notifications.show({ color: "green", message: "It is back in its place on the board.", title: "Email restored" });
    },
    onError: (error) => {
      notifications.show({ color: "red", message: error.message, title: "Restore failed" });
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
        message: `Status changed to ${formatEmailReviewStatus(response.review_status)}.`,
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

  const { createCommentMutation, createReplyMutation, resolveCommentMutation } =
    useCommentMutations(emailId);

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

  const invalidateEmailContent = () => {
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
    void queryClient.invalidateQueries({ queryKey: sharedTextsQueryKey });
  };

  const saveEditableFieldsMutation = useMutation({
    mutationFn: (payload: UpdateEditableFieldsPayload) =>
      updateEmailEditableFields(emailId, payload),
    onSuccess: invalidateEmailContent,
  });

  // A new block arrives in the translations with the EN text, so they need
  // a refetch as well; ["emails"] covers them.
  const saveLayoutMutation = useMutation({
    mutationFn: (layout: LayoutItem[]) => updateEmailLayout(emailId, layout),
    onSuccess: invalidateEmailContent,
    onError: (error) => {
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Failed to save the blocks.",
        title: "Blocks not saved",
      });
    },
  });

  return {
    duplicateEmailMutation,
    archiveEmailMutation,
    restoreEmailMutation,
    reviewStatusMutation,
    planningFieldsMutation,
    areaApprovalMutation,
    createCommentMutation,
    resolveCommentMutation,
    createReplyMutation,
    restoreVersionMutation,
    saveEditableFieldsMutation,
    saveLayoutMutation,
  };
}
