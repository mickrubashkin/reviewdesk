import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader, Modal } from "@mantine/core";
import { notifications } from "@mantine/notifications";

import { fetchBoards } from "../../boards/api";
import {
  ArchiveEmailModal,
  DuplicateEmailModal,
} from "../../review/EmailReview/ReviewModals";
import { archiveEmail, duplicateEmail, fetchEmailDetail, fetchEmails } from "../api";

// The modals for "Duplicate as..." and "Archive" are shared with the review
// page; they need the full email, so it is loaded when the modal opens.
function useEmailDetail(emailId: string) {
  return useQuery({
    queryKey: ["emails", emailId, "review"],
    queryFn: () => fetchEmailDetail(emailId),
  });
}

export function CardDuplicateModal({
  emailId,
  onClose,
}: {
  emailId: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const detailQuery = useEmailDetail(emailId);
  const boardsQuery = useQuery({ queryKey: ["boards"], queryFn: fetchBoards });
  const emailsQuery = useQuery({ queryKey: ["emails"], queryFn: () => fetchEmails() });
  const duplicateMutation = useMutation({
    mutationFn: (variables: Parameters<typeof duplicateEmail>) =>
      duplicateEmail(...variables),
    onSuccess: (created) => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: `Created "${created.title}".`,
        title: "Email duplicated",
      });
    },
  });

  if (!detailQuery.data) {
    return (
      <Modal centered opened title="Duplicate as..." onClose={onClose}>
        <Loader size="sm" />
      </Modal>
    );
  }

  return (
    <DuplicateEmailModal
      boards={boardsQuery.data ?? []}
      boardsLoading={boardsQuery.isLoading}
      email={detailQuery.data}
      emails={emailsQuery.data ?? []}
      emailsLoading={emailsQuery.isLoading}
      error={duplicateMutation.error}
      isSubmitting={duplicateMutation.isPending}
      onClose={onClose}
      onResetError={() => duplicateMutation.reset()}
      onSubmit={async (sourceEmailId, payload) => {
        await duplicateMutation.mutateAsync([sourceEmailId, payload]);
        onClose();
      }}
    />
  );
}

export function CardArchiveModal({
  emailId,
  onClose,
}: {
  emailId: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const detailQuery = useEmailDetail(emailId);
  const archiveMutation = useMutation({
    mutationFn: archiveEmail,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      notifications.show({
        color: "green",
        message: "It moved to the board's Archive, where it can be restored.",
        title: "Email archived",
      });
    },
    onError: () =>
      notifications.show({
        color: "red",
        message: "Try again or check that you have admin access.",
        title: "Archive failed",
      }),
  });

  if (!detailQuery.data) {
    return (
      <Modal centered opened title="Archive email" onClose={onClose}>
        <Loader size="sm" />
      </Modal>
    );
  }

  return (
    <ArchiveEmailModal
      email={detailQuery.data}
      isSubmitting={archiveMutation.isPending}
      opened
      onClose={onClose}
      onSubmit={async (id) => {
        await archiveMutation.mutateAsync(id);
        onClose();
      }}
    />
  );
}
