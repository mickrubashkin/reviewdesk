import {
  Alert,
  Badge,
  Card,
  Group,
  ScrollArea,
  SimpleGrid,
  Stack,
  Text,
} from "@mantine/core";

import {
  analysisCheckLabels,
  checkStatusMeta,
  priorityMeta,
  streamCheckOrder,
  verdictMeta,
} from "../analysisMeta";
import type { StreamPreview } from "../streamPreview";
import type { EmailAnalysis, EmailRecommendation } from "../types";
import styles from "../EmailPreviewDrawer.module.css";

type AnalysisPanelProps = {
  analysis: EmailAnalysis | null;
  error: string | null;
  isStreaming: boolean;
  preview: StreamPreview;
  showError: boolean;
};

export function AnalysisPanel({
  analysis,
  error,
  isStreaming,
  preview,
  showError,
}: AnalysisPanelProps) {
  if (showError) {
    return (
      <Alert color="red" title="Failed to stream AI analysis">
        {error ?? "Check that the API has OpenAI configured and try again."}
      </Alert>
    );
  }

  if (!analysis) {
    return (
      <Card className={styles.analysisCard} withBorder padding="md">
        <Stack gap="md">
          <Group justify="space-between" align="flex-start">
            <Stack gap={2}>
              <Group gap="xs">
                <Text fw={600}>AI analysis</Text>
                <span className={styles.streamPulse} />
              </Group>
              <Text size="sm" c="dimmed">
                Building structured review
              </Text>
            </Stack>
            <Badge radius="sm" variant="light" color="blue">
              {isStreaming ? "Streaming" : "Finalizing"}
            </Badge>
          </Group>

          <SimpleGrid cols={{ base: 1, sm: 3 }} spacing="xs">
            <div className={styles.streamStep}>
              <Text size="xs" fw={600}>
                Copy
              </Text>
              <Text size="xs" c="dimmed">
                Subject and body
              </Text>
            </div>
            <div className={styles.streamStep}>
              <Text size="xs" fw={600}>
                Checks
              </Text>
              <Text size="xs" c="dimmed">
                CTA and stage
              </Text>
            </div>
            <div className={styles.streamStep}>
              <Text size="xs" fw={600}>
                Result
              </Text>
              <Text size="xs" c="dimmed">
                Fixes
              </Text>
            </div>
          </SimpleGrid>

          <ScrollArea.Autosize mah={520} className={styles.streamOutput}>
            <StreamPreviewContent preview={preview} isStreaming={isStreaming} />
          </ScrollArea.Autosize>
        </Stack>
      </Card>
    );
  }

  return (
    <Card className={styles.analysisCard} withBorder padding="md">
      <Stack gap="sm">
        <Group justify="space-between" align="flex-start">
          <Stack gap={2}>
            <Text fw={600}>AI analysis</Text>
            <Text size="sm" c="dimmed">
              {analysis.summary}
            </Text>
          </Stack>
          <Group gap="xs">
            <Badge
              size="lg"
              radius="sm"
              variant="light"
              color={verdictMeta[analysis.verdict].color}
            >
              {verdictMeta[analysis.verdict].label}
            </Badge>
            <Badge size="lg" radius="sm" variant="light">
              {analysis.score}/10
            </Badge>
          </Group>
        </Group>

        <SimpleGrid cols={{ base: 2, sm: 2 }} spacing="xs">
          {Object.entries(analysis.checks).map(([check, status]) => (
            <Group
              key={check}
              className={styles.analysisCheck}
              justify="space-between"
              gap="xs"
            >
              <Text size="xs" c="dimmed">
                {analysisCheckLabels[check as keyof typeof analysisCheckLabels]}
              </Text>
              <Badge
                size="xs"
                radius="sm"
                variant="light"
                color={checkStatusMeta[status].color}
              >
                {checkStatusMeta[status].label}
              </Badge>
            </Group>
          ))}
        </SimpleGrid>

        <Stack gap="xs">
          {analysis.recommendations.map((recommendation, index) => (
            <Stack
              key={`${recommendation.title}-${index}`}
              className={styles.recommendationItem}
              gap={4}
            >
              <Group gap="xs">
                <Badge
                  size="xs"
                  radius="sm"
                  variant="light"
                  color={priorityMeta[recommendation.priority].color}
                >
                  {priorityMeta[recommendation.priority].label}
                </Badge>
                <Text size="sm" fw={600}>
                  {recommendation.title}
                </Text>
              </Group>
              <Text size="sm" c="dimmed">
                {recommendation.details}
              </Text>
            </Stack>
          ))}
        </Stack>
      </Stack>
    </Card>
  );
}

export default AnalysisPanel;

function StreamPreviewContent({
  isStreaming,
  preview,
}: {
  isStreaming: boolean;
  preview: StreamPreview;
}) {
  const hasContent =
    preview.summary ||
    preview.score !== null ||
    preview.verdict ||
    Object.keys(preview.checks).length > 0 ||
    preview.recommendations.length > 0;

  if (!hasContent) {
    return (
      <Stack gap={4} className={styles.streamHumanOutput}>
        <Text size="sm" fw={600}>
          Preparing the review
          {isStreaming ? <span className={styles.streamCursor}>...</span> : null}
        </Text>
        <Text size="sm" c="dimmed">
          The model is reading the email and separating copy, CTA, stage fit, and recommendations.
        </Text>
      </Stack>
    );
  }

  return (
    <Stack gap="sm" className={styles.streamHumanOutput}>
      {preview.summary ? (
        <Stack gap={2}>
          <Text size="xs" c="dimmed">
            Summary
          </Text>
          <Text size="sm">{preview.summary}</Text>
        </Stack>
      ) : null}

      <Group gap="xs">
        {preview.verdict ? (
          <Badge radius="sm" variant="light" color={verdictMeta[preview.verdict].color}>
            {verdictMeta[preview.verdict].label}
          </Badge>
        ) : null}
        {preview.score !== null ? (
          <Badge radius="sm" variant="light">
            {preview.score}/10
          </Badge>
        ) : null}
        {isStreaming ? (
          <Badge radius="sm" variant="dot" color="blue">
            Writing
          </Badge>
        ) : null}
      </Group>

      {Object.keys(preview.checks).length > 0 ? (
        <SimpleGrid cols={{ base: 2, sm: 3 }} spacing="xs">
          {streamCheckOrder.map((check) => {
            const status = preview.checks[check];
            if (!status) {
              return null;
            }

            return (
              <Group
                key={check}
                className={styles.analysisCheck}
                justify="space-between"
                gap="xs"
              >
                <Text size="xs" c="dimmed">
                  {analysisCheckLabels[check]}
                </Text>
                <Badge
                  size="xs"
                  radius="sm"
                  variant="light"
                  color={checkStatusMeta[status].color}
                >
                  {checkStatusMeta[status].label}
                </Badge>
              </Group>
            );
          })}
        </SimpleGrid>
      ) : null}

      {preview.recommendations.length > 0 ? (
        <Stack gap="xs">
          <Text size="xs" c="dimmed">
            Recommendations
          </Text>
          {preview.recommendations.map((recommendation, index) => (
            <RecommendationPreview
              key={`${recommendation.title ?? "draft"}-${index}`}
              recommendation={recommendation}
            />
          ))}
        </Stack>
      ) : null}
    </Stack>
  );
}

function RecommendationPreview({
  recommendation,
}: {
  recommendation: Partial<EmailRecommendation>;
}) {
  return (
    <Stack gap={3}>
      <Group gap="xs">
        {recommendation.priority ? (
          <Badge
            size="xs"
            radius="sm"
            variant="light"
            color={priorityMeta[recommendation.priority].color}
          >
            {priorityMeta[recommendation.priority].label}
          </Badge>
        ) : null}
        <Text size="sm" fw={600}>
          {recommendation.title ?? "Draft recommendation"}
        </Text>
      </Group>
      {recommendation.details ? (
        <Text size="sm" c="dimmed">
          {recommendation.details}
        </Text>
      ) : null}
    </Stack>
  );
}
