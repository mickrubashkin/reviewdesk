import { type FormEvent, useState } from "react";
import { Alert, Button, Group, Modal, Stack, Text, TextInput } from "@mantine/core";

import { ApiError } from "../../../shared/api";
import type { Board, CreateBoardPayload } from "../../emails/types";

export function CreateBoardModal({
  error,
  isSubmitting,
  sourceBoard,
  onClose,
  onResetError,
  onSubmit,
}: {
  error: Error | null;
  isSubmitting: boolean;
  sourceBoard: Board;
  onClose: () => void;
  onResetError: () => void;
  onSubmit: (payload: CreateBoardPayload) => Promise<Board>;
}) {
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);
  const isConflict = error instanceof ApiError && error.status === 409;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) {
      setNameError("Name is required");
      return;
    }

    setNameError(null);
    try {
      await onSubmit({
        key: key.trim() || undefined,
        name: trimmedName,
        source_board_key: sourceBoard.key,
      });
    } catch {
      // React Query stores the error; keep the modal open and show it inline.
    }
  };

  return (
    <Modal centered opened title="New board" onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <Stack gap="sm">
          {error ? (
            <Alert color="red" title="Could not create board">
              {isConflict
                ? "A board with this key already exists."
                : "Try again or check that the API server is reachable."}
            </Alert>
          ) : null}
          <Text size="sm" c="dimmed">
            Stages will be copied from {sourceBoard.name}.
          </Text>
          <TextInput
            data-autofocus
            disabled={isSubmitting}
            error={nameError}
            label="Name"
            placeholder="Activation"
            value={name}
            onChange={(event) => {
              onResetError();
              setName(event.currentTarget.value);
              if (nameError) {
                setNameError(null);
              }
            }}
          />
          <TextInput
            disabled={isSubmitting}
            label="Key"
            placeholder="activation"
            value={key}
            onChange={(event) => {
              onResetError();
              setKey(event.currentTarget.value);
            }}
          />
          <Group justify="flex-end" mt="xs">
            <Button
              disabled={isSubmitting}
              type="button"
              variant="default"
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button loading={isSubmitting} type="submit">
              Create board
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}


