import { Alert, Anchor, Badge, Group, Loader, SegmentedControl, Stack, Text, Tooltip } from "@mantine/core";
import { BarChart } from "@mantine/charts";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { fetchEmailSends, type EmailSends } from "../../../portal/api";
import { buildSendsChart, formatDay, seriesTotals } from "../../../portal/emailSends";
import { formatCommentDate } from "../EmailReview.helpers";
import styles from "../EmailReview.module.css";

const periods = ["30", "90", "180"];

// SendsSection charts the email's sends per day as the portal sync saw them,
// split by the text that went out, so a re-upload shows as a new color taking
// over from the old one.
export function SendsSection({ canOpenPortalSync, emailId }: { canOpenPortalSync: boolean; emailId: string }) {
  const [days, setDays] = useState("90");
  const query = useQuery({
    queryKey: ["emails", emailId, "sends", days],
    queryFn: () => fetchEmailSends(emailId, Number(days)),
  });
  const chart = useMemo(() => (query.data ? buildSendsChart(query.data) : null), [query.data]);
  const total = query.data?.texts.reduce((sum, text) => sum + text.sends, 0) ?? 0;
  // The subject tells texts apart only when a re-upload changed it.
  const subjectsDiffer = new Set(query.data?.texts.map((text) => text.subject)).size > 1;

  return (
    <Stack className={styles.handoffSection} gap="xs">
      <Group justify="space-between" gap="xs" wrap="nowrap">
        <Text fw={700} size="sm">
          Sends
        </Text>
        <SegmentedControl
          aria-label="Period"
          data={periods.map((value) => ({ value, label: `${value} days` }))}
          size="xs"
          value={days}
          onChange={setDays}
        />
      </Group>
      {query.isLoading ? (
        <Loader size="xs" />
      ) : query.isError || !chart || !query.data ? (
        <Text c="red" size="xs">
          Sends could not be loaded.
        </Text>
      ) : total === 0 ? (
        <Text c="dimmed" size="xs">
          The portal sync saw no sends of this email in the last {days} days.
        </Text>
      ) : (
        <>
          <Text c="dimmed" size="xs">
            {total} sends in the last {days} days, per day and by the text that went out.
          </Text>
          <BarChart
            barProps={{ radius: 2 }}
            data={chart.rows.map((row) => ({ ...row, day: formatDay(row.date as string) }))}
            dataKey="day"
            h={160}
            series={chart.series.map(({ key, label, color }) => ({ name: key, label, color }))}
            tickLine="none"
            tooltipAnimationDuration={0}
            type="stacked"
            xAxisProps={{ interval: "preserveStartEnd", minTickGap: 24 }}
            yAxisProps={{ allowDecimals: false, tickCount: 3, width: 32 }}
          />
          {/* The legend, with the numbers behind each color. */}
          <Stack gap={4}>
            {chart.series.map((series) => {
              const { sends, lastSentAt } = seriesTotals(series);
              const [text] = series.texts;
              return (
                <Group key={series.key} gap={6} wrap="nowrap" align="flex-start">
                  <span aria-hidden="true" className={styles.sendsSwatch} style={{ background: series.color }} />
                  <Text size="xs">
                    {series.label}: {sends} {sends === 1 ? "send" : "sends"}, last {formatCommentDate(lastSentAt)}
                    {series.texts.length === 1 && subjectsDiffer ? ` · ${text.subject}` : ""}
                    {canOpenPortalSync && series.texts.length === 1 ? (
                      <>
                        {" · "}
                        <Anchor
                          component={Link}
                          size="xs"
                          to={`/admin/portal?board=${encodeURIComponent(text.board_key)}&group=${encodeURIComponent(text.group_id)}`}
                        >
                          Portal sync
                        </Anchor>
                      </>
                    ) : null}
                  </Text>
                </Group>
              );
            })}
          </Stack>
          <SendDeals deals={query.data.deals} />
          <SendStages stages={query.data.stages} />
        </>
      )}
    </Stack>
  );
}

function plural(count: number, one: string, many: string) {
  return `${count} ${count === 1 ? one : many}`;
}

// SendDeals sets the sends against the deals behind them. A deal getting the
// same text twice within a day is what two robots sending the email would do.
function SendDeals({ deals }: { deals: EmailSends["deals"] }) {
  return (
    <>
      <Text c="dimmed" size="xs">
        Sent to {plural(deals.deals, "deal", "deals")}
        {deals.repeated > 0
          ? `; ${plural(deals.repeated, "deal", "deals")} got it more than once (the deal came back to the stage, or a robot repeats it)`
          : ""}
        .
      </Text>
      {deals.doubled > 0 ? (
        <Alert color="yellow" p="xs">
          <Text size="xs">
            {plural(deals.doubled, "deal", "deals")} got the same text twice within a day. Two robots may be sending
            this email.
          </Text>
        </Alert>
      ) : null}
    </>
  );
}

// SendStages counts the sends by the deal's stage when the email went out,
// marking stages that mostly send the emails of another board stage.
function SendStages({ stages }: { stages: EmailSends["stages"] }) {
  return (
    <Stack gap={2}>
      <Text fw={600} size="xs">
        By deal stage
      </Text>
      {stages.map((stage) => (
        <Group key={stage.stage_id} gap={6} wrap="nowrap">
          <Text size="xs">
            {stage.name || "Stage unknown"}: {plural(stage.sends, "send", "sends")}
          </Text>
          {stage.other ? (
            <Tooltip
              label="On this board, most sends from this deal stage are emails of another stage. Check the robot that sends this email from it."
              multiline
              w={260}
            >
              <Badge color="orange" radius="sm" size="xs" variant="light">
                other stage
              </Badge>
            </Tooltip>
          ) : null}
        </Group>
      ))}
    </Stack>
  );
}
