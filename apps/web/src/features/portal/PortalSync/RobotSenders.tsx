import { Alert, Button, Group, Paper, Select, Table, Text, Title } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { fetchRobotSenders, saveRobotSender, type PortalSender } from "../api";
import { suggestionsKey } from "../uploadSuggestions";

function senderLabel(sender: PortalSender) {
  const name = sender.name || `User ${sender.user_id}`;
  return sender.email ? `${name} (${sender.email})` : name;
}

// RobotSenders pairs each robot of the board (a region manager) with the
// portal user whose deals it sends to. The portal names a send's sender, not
// its robot: only paired robots get their sends counted.
// run names the latest sync and its status, to reload when one finishes.
export function RobotSenders({ boardKey, run }: { boardKey: string; run: string }) {
  const queryClient = useQueryClient();
  const queryKey = ["portal-robot-senders", boardKey];
  const query = useQuery({ queryKey: [...queryKey, run], queryFn: () => fetchRobotSenders(boardKey) });
  const mutation = useMutation({
    mutationFn: ({ robot, userID }: { robot: string; userID: number | null }) => saveRobotSender(boardKey, robot, userID),
    onError: (error) => notifications.show({ color: "red", message: error.message, title: "Not saved" }),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey });
      void queryClient.invalidateQueries({ queryKey: suggestionsKey });
    },
  });

  const data = query.data;
  if (query.isError) {
    return <Alert color="red">Failed to load the robots.</Alert>;
  }
  if (!data || data.robots.length === 0) {
    return null;
  }
  const byID = new Map(data.senders.map((sender) => [sender.user_id, sender]));
  const pairedTo = new Map(
    data.robots.flatMap((robot) => (robot.portal_user_id !== null ? [[robot.portal_user_id, robot.robot] as const] : []))
  );

  return (
    <>
      <Title order={4}>Robots and senders</Title>
      <Text c="dimmed" size="sm">
        A robot sends for the deals of one manager. Pair each robot with the portal user who is responsible for those
        deals: then the portal's sends show which robot has which version.
        {data.senders.length === 0 ? " Senders appear after the next sync." : ""}
        {data.unpaired_sends > 0 ? ` ${data.unpaired_sends} loaded sends have no paired robot yet.` : ""}
      </Text>
      <Paper withBorder>
        <Table striped>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Robot</Table.Th>
              <Table.Th>Portal user</Table.Th>
              <Table.Th>Sends</Table.Th>
              <Table.Th title="Upload marks with a send of the email from this robot's deals, at any version">
                Marks seen sent
              </Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {data.robots.map((robot) => {
              const suggested = robot.suggested_user_id !== null ? byID.get(robot.suggested_user_id) : undefined;
              return (
                <Table.Tr key={robot.robot}>
                  <Table.Td>{robot.robot}</Table.Td>
                  <Table.Td>
                    <Group gap="xs" wrap="nowrap">
                      <Select
                        aria-label={`Portal user of ${robot.robot}'s robot`}
                        clearable
                        data={data.senders.map((sender) => {
                          const other = pairedTo.get(sender.user_id);
                          return {
                            disabled: other !== undefined && other !== robot.robot,
                            label: `${senderLabel(sender)} · ${sender.sends} sends${other && other !== robot.robot ? ` · ${other}` : ""}`,
                            value: String(sender.user_id),
                          };
                        })}
                        disabled={mutation.isPending}
                        miw={320}
                        placeholder={suggested ? `Suggested: ${senderLabel(suggested)}` : "Not paired"}
                        searchable
                        size="xs"
                        value={robot.portal_user_id !== null ? String(robot.portal_user_id) : null}
                        onChange={(value) =>
                          mutation.mutate({ robot: robot.robot, userID: value !== null ? Number(value) : null })
                        }
                      />
                      {suggested ? (
                        <Button
                          size="compact-xs"
                          variant="light"
                          onClick={() => mutation.mutate({ robot: robot.robot, userID: suggested.user_id })}
                        >
                          Pair
                        </Button>
                      ) : null}
                    </Group>
                  </Table.Td>
                  <Table.Td>{robot.portal_user_id !== null ? robot.sends : "–"}</Table.Td>
                  <Table.Td>
                    {robot.portal_user_id !== null ? `${robot.marks_seen_sent} of ${robot.marks}` : `– of ${robot.marks}`}
                  </Table.Td>
                </Table.Tr>
              );
            })}
          </Table.Tbody>
        </Table>
      </Paper>
    </>
  );
}
