import { type FormEvent, useState } from "react";
import { ActionIcon, Alert, Button, Group, Loader, Stack, TextInput, Tooltip } from "@mantine/core";
import { ArrowLeftIcon, ArrowRightIcon, PencilSimpleIcon, PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createBoardApprovalArea, deleteBoardApprovalArea, fetchBoardApprovalAreas, reorderBoardApprovalAreas, updateBoardApprovalArea } from "../api";
import type { Board, BoardApprovalArea } from "../../emails/types";

import styles from "../../../App.module.css";

export function BoardApprovalAreasManager({ board }: { board: Board }) {
  const queryClient = useQueryClient();
  const [newAreaName, setNewAreaName] = useState("");
  const [areaNames, setAreaNames] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);

  const areasQuery = useQuery({
    queryKey: ["board-approval-areas", board.key],
    queryFn: () => fetchBoardApprovalAreas(board.key),
  });
  const areas = areasQuery.data ?? [];

  const refreshAreas = () => {
    setFormError(null);
    void queryClient.invalidateQueries({
      queryKey: ["board-approval-areas", board.key],
    });
  };

  const handleError = () => {
    setFormError("Could not update approval areas. Try again or check admin access.");
  };

  const createMutation = useMutation({
    mutationFn: () =>
      createBoardApprovalArea(board.key, {
        name: newAreaName.trim(),
        required: true,
      }),
    onSuccess: () => {
      setNewAreaName("");
      refreshAreas();
    },
    onError: handleError,
  });
  const updateMutation = useMutation({
    mutationFn: ({
      area,
      name,
      required,
    }: {
      area: BoardApprovalArea;
      name?: string;
      required?: boolean;
    }) =>
      updateBoardApprovalArea(board.key, area.key, {
        name,
        required,
      }),
    onSuccess: refreshAreas,
    onError: handleError,
  });
  const deleteMutation = useMutation({
    mutationFn: (area: BoardApprovalArea) =>
      deleteBoardApprovalArea(board.key, area.key),
    onSuccess: refreshAreas,
    onError: handleError,
  });
  const reorderMutation = useMutation({
    mutationFn: (nextAreas: string[]) =>
      reorderBoardApprovalAreas(board.key, { areas: nextAreas }),
    onSuccess: refreshAreas,
    onError: handleError,
  });

  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    deleteMutation.isPending ||
    reorderMutation.isPending;

  const handleAddArea = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!newAreaName.trim()) {
      setFormError("Approval area name is required.");
      return;
    }
    createMutation.mutate();
  };

  const moveArea = (areaIndex: number, direction: -1 | 1) => {
    const nextIndex = areaIndex + direction;
    if (nextIndex < 0 || nextIndex >= areas.length) {
      return;
    }
    const nextAreas = areas.map((area) => area.key);
    [nextAreas[areaIndex], nextAreas[nextIndex]] = [
      nextAreas[nextIndex],
      nextAreas[areaIndex],
    ];
    reorderMutation.mutate(nextAreas);
  };

  if (areasQuery.isLoading) {
    return (
      <Stack align="center" py="md">
        <Loader size="sm" />
      </Stack>
    );
  }

  if (areasQuery.isError) {
    return (
      <Alert color="red" title="Could not load approval areas">
        Try refreshing the page or check the API server.
      </Alert>
    );
  }

  return (
    <Stack gap="md">
      {formError ? (
        <Alert color="red" title="Could not update approval areas">
          {formError}
        </Alert>
      ) : null}

      <form onSubmit={handleAddArea}>
        <Group align="flex-end" gap="xs" wrap="nowrap">
          <TextInput
            disabled={isSubmitting}
            label="New approval area"
            placeholder="Compliance"
            value={newAreaName}
            onChange={(event) => {
              setFormError(null);
              setNewAreaName(event.currentTarget.value);
            }}
          />
          <Button
            disabled={isSubmitting || !newAreaName.trim()}
            leftSection={<PlusIcon aria-hidden="true" size={16} />}
            type="submit"
          >
            Add
          </Button>
        </Group>
      </form>

      <Stack gap="xs">
        {areas.map((area, areaIndex) => {
          const areaName = areaNames[area.key] ?? area.name;
          const isNameChanged = areaName.trim() !== area.name;

          return (
            <Group className={styles.stageManagerRow} gap="xs" key={area.id} wrap="nowrap">
              <TextInput
                className={styles.stageManagerName}
                disabled={isSubmitting}
                value={areaName}
                onChange={(event) => {
                  const nextAreaName = event.currentTarget.value;
                  setFormError(null);
                  setAreaNames((current) => ({
                    ...current,
                    [area.key]: nextAreaName,
                  }));
                }}
              />
              <Button
                disabled={isSubmitting}
                size="xs"
                variant={area.required ? "filled" : "light"}
                onClick={() =>
                  updateMutation.mutate({
                    area,
                    required: !area.required,
                  })
                }
              >
                {area.required ? "Required" : "Optional"}
              </Button>
              <Tooltip label="Move area left">
                <ActionIcon
                  aria-label="Move area left"
                  disabled={isSubmitting || areaIndex === 0}
                  variant="default"
                  onClick={() => moveArea(areaIndex, -1)}
                >
                  <ArrowLeftIcon aria-hidden="true" size={16} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Move area right">
                <ActionIcon
                  aria-label="Move area right"
                  disabled={isSubmitting || areaIndex === areas.length - 1}
                  variant="default"
                  onClick={() => moveArea(areaIndex, 1)}
                >
                  <ArrowRightIcon aria-hidden="true" size={16} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Rename area">
                <ActionIcon
                  aria-label="Rename area"
                  disabled={isSubmitting || !areaName.trim() || !isNameChanged}
                  variant="default"
                  onClick={() =>
                    updateMutation.mutate({
                      area,
                      name: areaName.trim(),
                    })
                  }
                >
                  <PencilSimpleIcon aria-hidden="true" size={16} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Archive area">
                <ActionIcon
                  aria-label="Archive area"
                  color="red"
                  disabled={isSubmitting}
                  variant="light"
                  onClick={() => deleteMutation.mutate(area)}
                >
                  <TrashIcon aria-hidden="true" size={16} />
                </ActionIcon>
              </Tooltip>
            </Group>
          );
        })}
      </Stack>
    </Stack>
  );
}


