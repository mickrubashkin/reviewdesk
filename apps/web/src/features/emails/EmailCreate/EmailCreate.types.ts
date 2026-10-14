import type { EmailVariant } from "../types";

export type CreateEmailFormState = {
  sequence: string;
  eventGroupKey: string;
  title: string;
  subject: string;
  preheader: string;
  sendTiming: string;
  stage: string;
  sortOrder: number;
  language: string;
  variant: EmailVariant;
  adaptationLabel: string;
  originalHTML: string;
};

export type EventGroupOption = {
  value: string;
  label: string;
  title: string;
  sortOrder: number;
  sendTiming: string | null;
  adaptationLabel: string;
};

export const newEventGroupValue = "__new_event__";

export const initialFormState: CreateEmailFormState = {
  sequence: "onboarding",
  eventGroupKey: newEventGroupValue,
  title: "",
  subject: "",
  preheader: "",
  sendTiming: "",
  stage: "",
  sortOrder: 0,
  language: "en",
  variant: "v1",
  adaptationLabel: "Default",
  originalHTML: "",
};
