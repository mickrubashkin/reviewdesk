import { Group, Select, Stack, Text } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { ApiError } from "../../../../shared/api";
import { useBoardRegions } from "../../../boards/api";
import { updateEmailRegion } from "../../../emails/api";
import type { EmailDetail } from "../../../emails/types";
import { HelpLink } from "../../../help/HelpLink";

// RegionField shows which region of its board the email belongs to; a
// manager of the email changes it. Boards without regions show nothing.
export function RegionField({ canEdit, email }: { canEdit: boolean; email: EmailDetail }) {
  const queryClient = useQueryClient();
  const regionsQuery = useBoardRegions(email.sequence);
  const regions = regionsQuery.data ?? [];
  const mutation = useMutation({
    mutationFn: (regionKey: string) => updateEmailRegion(email.id, regionKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
      void queryClient.invalidateQueries({ queryKey: ["email-activity", email.id] });
    },
    onError: (error) =>
      notifications.show({
        color: "red",
        message: error instanceof ApiError ? error.message : "Try again.",
        title: "Region not saved",
      }),
  });

  if (regions.length === 0) {
    return null;
  }
  const current = regions.find((region) => region.key === email.region_key);
  return (
    <Stack gap={4}>
      <Select
        allowDeselect={false}
        data={[
          { label: "No region", value: "" },
          ...regions.map((region) => ({
            label: `${region.name}${
              region.reference && !region.name.toLowerCase().includes("reference") ? " · reference" : ""
            } (${region.language.toUpperCase()})`,
            value: region.key,
          })),
        ]}
        disabled={!canEdit || mutation.isPending}
        label={
          <Group component="span" gap={4} wrap="nowrap">
            Region
            <HelpLink topic="regions" />
          </Group>
        }
        value={mutation.isPending ? mutation.variables : (email.region_key ?? "")}
        onChange={(value) => mutation.mutate(value ?? "")}
      />
      {current && current.managers.length > 0 ? (
        <Text c="dimmed" size="xs">
          Robots of {current.managers.join(", ")}
        </Text>
      ) : null}
    </Stack>
  );
}
