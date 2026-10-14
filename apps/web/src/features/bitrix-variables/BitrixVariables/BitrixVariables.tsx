import { useState } from "react";
import {
  ActionIcon,
  Alert,
  Button,
  Group,
  Loader,
  Paper,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { PlusIcon, TrashIcon } from "@phosphor-icons/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";

import { AdminTableHeader } from "../../admin-table/AdminTableHeader";
import type { UserRole } from "../../emails/types";
import {
  bitrixVariablesQueryKey,
  saveBitrixVariables,
  useBitrixVariables,
  type BitrixVariable,
} from "../api";
import styles from "../../auth-events/AuthEvents/AuthEvents.module.css";

const emptyVariable: BitrixVariable = { label: "", expression: "", example: "", note: "" };

export function BitrixVariables({ currentUserRole }: { currentUserRole: UserRole }) {
  const variablesQuery = useBitrixVariables();

  if (currentUserRole !== "super_admin") {
    return <Navigate replace to="/" />;
  }

  return (
    <div className={styles.page}>
      <AdminTableHeader
        subtitle="Values the CRM robot fills in when it sends an email, such as the manager's booking link. Editors insert them into email fields on any board."
        title="Bitrix variables"
      />
      <Stack maw={860} p="md">
        {variablesQuery.isLoading ? (
          <Loader />
        ) : variablesQuery.isError || !variablesQuery.data ? (
          <Alert color="red">Failed to load the variables.</Alert>
        ) : (
          <VariablesForm key={variablesQuery.dataUpdatedAt} variables={variablesQuery.data} />
        )}
      </Stack>
    </div>
  );
}

function VariablesForm({ variables }: { variables: BitrixVariable[] }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<BitrixVariable[]>(variables);

  const saveMutation = useMutation({
    mutationFn: () => saveBitrixVariables(draft),
    onSuccess: (saved) => {
      queryClient.setQueryData(bitrixVariablesQueryKey, saved);
      notifications.show({ color: "green", message: "Editors can insert them now.", title: "Variables saved" });
    },
    onError: (error) =>
      notifications.show({ color: "red", message: error.message, title: "Save failed" }),
  });

  const update = (index: number, patch: Partial<BitrixVariable>) =>
    setDraft((current) => current.map((item, i) => (i === index ? { ...item, ...patch } : item)));

  return (
    <Stack gap="md">
      <Text c="dimmed" size="sm">
        Copy the expression from the robot&apos;s email editor in Bitrix24 (insert a field, then
        copy the text it adds), for example {"{{Responsible person > Booking link}}"}. The service
        keeps it in the email unchanged; the preview shows the example instead.
      </Text>

      {draft.length === 0 ? (
        <Text c="dimmed" size="sm">
          No variables yet.
        </Text>
      ) : null}

      {draft.map((variable, index) => (
        <Paper key={index} p="sm" withBorder>
          <Group align="flex-start" wrap="nowrap">
            <Stack flex={1} gap={6}>
              <Group grow>
                <TextInput
                  label="Name"
                  placeholder="Booking link"
                  value={variable.label}
                  onChange={(event) => update(index, { label: event.currentTarget.value })}
                />
                <TextInput
                  label="Bitrix24 expression"
                  placeholder="{{Responsible person > Booking link}}"
                  styles={{ input: { fontFamily: "var(--mantine-font-family-monospace)" } }}
                  value={variable.expression}
                  onChange={(event) => update(index, { expression: event.currentTarget.value })}
                />
              </Group>
              <Group grow>
                <TextInput
                  description="Shown in the preview"
                  label="Example value"
                  placeholder="https://calendly.com/..."
                  value={variable.example}
                  onChange={(event) => update(index, { example: event.currentTarget.value })}
                />
                <TextInput
                  description="Optional hint for editors"
                  label="Note"
                  value={variable.note}
                  onChange={(event) => update(index, { note: event.currentTarget.value })}
                />
              </Group>
            </Stack>
            <ActionIcon
              aria-label="Remove variable"
              color="red"
              mt={28}
              variant="subtle"
              onClick={() => setDraft((current) => current.filter((_, i) => i !== index))}
            >
              <TrashIcon aria-hidden="true" size={16} />
            </ActionIcon>
          </Group>
        </Paper>
      ))}

      <Group justify="space-between">
        <Button
          leftSection={<PlusIcon aria-hidden="true" size={14} />}
          size="xs"
          variant="light"
          onClick={() => setDraft((current) => [...current, emptyVariable])}
        >
          Add variable
        </Button>
        <Button loading={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
          Save
        </Button>
      </Group>
    </Stack>
  );
}
