import { useEffect, useRef } from "react";
import { List } from "@mantine/core";
import { notifications } from "@mantine/notifications";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { fetchBoardTranslationJobs } from "../../emails/api";

// useBoardTranslationJobs follows the board's AI translations into empty
// cells. It polls while one runs; when one finishes the board's emails are
// reloaded so the draft shows, and the person who started it is told how it
// went.
export function useBoardTranslationJobs(boardKey: string | undefined, currentUserEmail: string) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["translation-jobs", boardKey ?? ""],
    queryFn: () => fetchBoardTranslationJobs(boardKey ?? ""),
    enabled: Boolean(boardKey),
    refetchInterval: (q) => (q.state.data?.some((job) => job.status === "running") ? 4000 : false),
  });
  const jobs = query.data;

  // The jobs seen running, to tell when one ends.
  const running = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!jobs) return;
    const seen = running.current;
    running.current = new Set(jobs.filter((job) => job.status === "running").map((job) => job.id));
    if (!seen) return;
    const ended = jobs.filter((job) => job.status !== "running" && seen.has(job.id));
    if (ended.some((job) => job.status === "done")) {
      void queryClient.invalidateQueries({ queryKey: ["emails"] });
    }
    for (const job of ended) {
      if (job.started_by_email !== currentUserEmail) continue;
      const label = job.language.toUpperCase();
      if (job.status === "failed") {
        notifications.show({ color: "red", title: `${label} not translated`, message: job.error });
        continue;
      }
      const summary = job.result;
      notifications.show({
        color: "green",
        title: `${label} created`,
        message: `Draft translated by ${summary?.model ?? "AI"}${summary?.glossary.length ? `, ${summary.glossary.length} glossary terms` : ""}. Read it before marking it ready.`,
      });
      if (summary?.kept_english.length) {
        notifications.show({
          autoClose: false,
          color: "yellow",
          title: `${label}: ${summary.kept_english.length} fields left in English`,
          message: (
            <List size="xs">
              {summary.kept_english.map((field) => (
                <List.Item key={field.key}>
                  {field.key}: {field.problem}
                </List.Item>
              ))}
            </List>
          ),
        });
      }
    }
  }, [jobs, currentUserEmail, queryClient]);

  return jobs ?? [];
}
