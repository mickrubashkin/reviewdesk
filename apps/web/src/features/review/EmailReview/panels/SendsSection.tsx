import { Anchor, Group, Loader, SegmentedControl, Stack, Text } from "@mantine/core";
import { BarChart } from "@mantine/charts";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { fetchEmailSends } from "../../../portal/api";
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
      ) : query.isError || !chart ? (
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
        </>
      )}
    </Stack>
  );
}
