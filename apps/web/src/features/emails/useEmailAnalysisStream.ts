import { useEffect, useRef, useState } from "react";

import { analyzeEmailStream } from "./api";
import type { EmailAnalysis, EmailDetail } from "./types";

export type EmailAnalysisStreamStatus = "idle" | "streaming" | "done" | "error";

export function useEmailAnalysisStream(
  email: EmailDetail | undefined,
  opened: boolean,
  onAnalysisReady?: (analysis: EmailAnalysis) => void
) {
  const stopStreamRef = useRef<(() => void) | null>(null);
  const [streamEmailId, setStreamEmailId] = useState<string | null>(null);
  const [streamStatus, setStreamStatus] =
    useState<EmailAnalysisStreamStatus>("idle");
  const [streamText, setStreamText] = useState("");
  const [streamError, setStreamError] = useState<string | null>(null);
  const [streamAnalysis, setStreamAnalysis] = useState<EmailAnalysis | null>(null);

  useEffect(() => {
    stopStreamRef.current?.();
    stopStreamRef.current = null;
  }, [email?.id, opened]);

  useEffect(() => {
    return () => {
      stopStreamRef.current?.();
    };
  }, []);

  const analyze = (options: { refresh?: boolean } = {}) => {
    if (!email) {
      return;
    }

    stopStreamRef.current?.();
    setStreamEmailId(email.id);
    setStreamStatus("streaming");
    setStreamText("");
    setStreamError(null);
    setStreamAnalysis(null);

    stopStreamRef.current = analyzeEmailStream(
      email.id,
      {
        onDelta: (text) => {
          setStreamText((current) => current + text);
        },
        onResult: (analysis) => {
          setStreamAnalysis(analysis);
          onAnalysisReady?.(analysis);
        },
        onDone: () => {
          setStreamStatus("done");
          stopStreamRef.current = null;
        },
        onError: (error) => {
          setStreamStatus("error");
          setStreamError(error);
          stopStreamRef.current = null;
        },
      },
      options
    );
  };

  return {
    analyze,
    reanalyze: () => analyze({ refresh: true }),
    streamAnalysis,
    streamEmailId,
    streamError,
    streamStatus,
    streamText,
  };
}
