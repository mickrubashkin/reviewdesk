import { type FormEvent, useState } from "react";
import { ActionIcon, Alert, Badge, Button, Group, Modal, SegmentedControl, Stack, TextInput, Tooltip } from "@mantine/core";
import { ArrowLeftIcon, ArrowRightIcon, PencilSimpleIcon, PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation } from "@tanstack/react-query";

import { ApiError } from "../../../shared/api";
import { createBoardStage, deleteBoardStage, reorderBoardStages, updateBoardStage } from "../api";
import { formatStageName } from "../../emails/stages";
import type { Board } from "../../emails/types";

import { BoardApprovalAreasManager } from "./BoardApprovalAreasManager";
import styles from "../../../App.module.css";

export function ManageStagesModal({
  board,
  stageCounts,
  onClose,
  onMutated,
}: {
  board: Board;
  stageCounts: Map<string, number>;
  onClose: () => void;
  onMutated: () => void;
}) {
  const [activeSettingsTab, setActiveSettingsTab] = useState("stages");
  const [newStageName, setNewStageName] = useState("");
  const [stageNames, setStageNames] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const handleSuccess = (updatedBoard: Board) => {
    setFormError(null);
    setStageNames(
      Object.fromEntries(updatedBoard.stages.map((stage) => [stage, formatStageName(stage)]))
    );
    onMutated();
  };

  const handleError = (error: Error) => {
    if (error instanceof ApiError) {
      if (error.status === 409) {
        setFormError(error.message.includes("not empty")
          ? "Only empty stages can be deleted."
          : "A stage with this name already exists.");
        return;
      }
      if (error.status === 400) {
        setFormError("Stage order must contain the same stages exactly once.");
        return;
      }
    }
    setFormError("Could not update stages. Try again or check the API server.");
  };

  const addStageMutation = useMutation({
    mutationFn: () => createBoardStage(board.key, { name: newStageName.trim() }),
    onSuccess: (updatedBoard) => {
      setNewStageName("");
      handleSuccess(updatedBoard);
    },
    onError: handleError,
  });
  const renameStageMutation = useMutation({
    mutationFn: ({ stage, name }: { stage: string; name: string }) =>
      updateBoardStage(board.key, stage, { name }),
    onSuccess: handleSuccess,
    onError: handleError,
  });
  const deleteStageMutation = useMutation({
    mutationFn: (stage: string) => deleteBoardStage(board.key, stage),
    onSuccess: handleSuccess,
    onError: handleError,
  });
  const reorderStageMutation = useMutation({
    mutationFn: (stages: string[]) => reorderBoardStages(board.key, { stages }),
    onSuccess: handleSuccess,
    onError: handleError,
  });

  const isSubmitting =
    addStageMutation.isPending ||
    renameStageMutation.isPending ||
    deleteStageMutation.isPending ||
    reorderStageMutation.isPending;

  const handleAddStage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!newStageName.trim()) {
      setFormError("Stage name is required.");
      return;
    }
    addStageMutation.mutate();
  };

  const moveStage = (stageIndex: number, direction: -1 | 1) => {
    const nextIndex = stageIndex + direction;
    if (nextIndex < 0 || nextIndex >= board.stages.length) {
      return;
    }
    const nextStages = [...board.stages];
    [nextStages[stageIndex], nextStages[nextIndex]] = [
      nextStages[nextIndex],
      nextStages[stageIndex],
    ];
    reorderStageMutation.mutate(nextStages);
  };

  return (
    <Modal centered opened size="lg" title={`Board settings for ${board.name}`} onClose={onClose}>
      <Stack gap="md">
        <SegmentedControl
          data={[
            { label: "Stages", value: "stages" },
            { label: "Approval areas", value: "approval_areas" },
          ]}
          value={activeSettingsTab}
          onChange={(val) => setActiveSettingsTab(val)}
        />

        {activeSettingsTab === "stages" ? (
          <>
            {formError ? (
              <Alert color="red" title="Could not update stages">
                {formError}
              </Alert>
            ) : null}

            <form onSubmit={handleAddStage}>
              <Group align="flex-end" gap="xs" wrap="nowrap">
                <TextInput
                  disabled={isSubmitting}
                  label="New stage"
                  placeholder="Ready for QA"
                  value={newStageName}
                  onChange={(event) => {
                    setFormError(null);
                    setNewStageName(event.currentTarget.value);
                  }}
                />
                <Button
                  disabled={isSubmitting || !newStageName.trim()}
                  leftSection={<PlusIcon aria-hidden="true" size={16} />}
                  type="submit"
                >
                  Add
                </Button>
              </Group>
            </form>

            <Stack gap="xs">
              {board.stages.map((stage, stageIndex) => {
                const stageName = stageNames[stage] ?? formatStageName(stage);
                const emailCount = stageCounts.get(stage) ?? 0;
                const isNameChanged = stageName.trim() !== formatStageName(stage);

                return (
                  <Group className={styles.stageManagerRow} gap="xs" key={stage} wrap="nowrap">
                    <TextInput
                      className={styles.stageManagerName}
                      disabled={isSubmitting}
                      value={stageName}
                      onChange={(event) => {
                        const nextStageName = event.currentTarget.value;
                        setFormError(null);
                        setStageNames((current) => ({
                          ...current,
                          [stage]: nextStageName,
                        }));
                      }}
                    />
                    <Badge variant="light">{emailCount}</Badge>
                    <Tooltip label="Move stage left">
                      <ActionIcon
                        aria-label="Move stage left"
                        disabled={isSubmitting || stageIndex === 0}
                        variant="default"
                        onClick={() => moveStage(stageIndex, -1)}
                      >
                        <ArrowLeftIcon aria-hidden="true" size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label="Move stage right">
                      <ActionIcon
                        aria-label="Move stage right"
                        disabled={isSubmitting || stageIndex === board.stages.length - 1}
                        variant="default"
                        onClick={() => moveStage(stageIndex, 1)}
                      >
                        <ArrowRightIcon aria-hidden="true" size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label="Rename stage">
                      <ActionIcon
                        aria-label="Rename stage"
                        disabled={isSubmitting || !stageName.trim() || !isNameChanged}
                        variant="default"
                        onClick={() =>
                          renameStageMutation.mutate({
                            name: stageName.trim(),
                            stage,
                          })
                        }
                      >
                        <PencilSimpleIcon aria-hidden="true" size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label={emailCount > 0 ? "Only empty stages can be deleted" : "Delete stage"}>
                      <ActionIcon
                        aria-label="Delete stage"
                        color="red"
                        disabled={isSubmitting || emailCount > 0}
                        variant="light"
                        onClick={() => deleteStageMutation.mutate(stage)}
                      >
                        <TrashIcon aria-hidden="true" size={16} />
                      </ActionIcon>
                    </Tooltip>
                  </Group>
                );
              })}
            </Stack>
          </>
        ) : (
          <BoardApprovalAreasManager board={board} />
        )}

        <Group justify="flex-end">
          <Button disabled={isSubmitting} variant="default" onClick={onClose}>
            Close
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}


