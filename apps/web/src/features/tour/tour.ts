import type { AuthUser } from "../emails/types";
import { helpRole } from "../help/help";

// TOUR_VERSION goes up when the tour changes enough that everyone should see
// it again; the API keeps the version each person last finished or skipped.
export const TOUR_VERSION = 1;

export type TourStep = {
  // A data-tour value in the board header; without one the step is a
  // popover in the middle of the screen.
  target?: string;
  title: string;
  description: string;
};

type TourUser = Pick<AuthUser, "role" | "languages" | "managed_boards" | "tour_version">;

// shouldStartTour: the tour opens by itself once per version, never in the
// public demo, which has its own guide.
export function shouldStartTour(user: TourUser, demo: boolean) {
  return !demo && (user.tour_version ?? 0) < TOUR_VERSION;
}

// tourSteps walks a new person through the board header, saying for their
// role what each part is for. It ends on the account menu, where the help
// and the tour stay.
export function tourSteps(user: TourUser): TourStep[] {
  const role = helpRole(user);
  const edits = user.role !== "reviewer" || (user.managed_boards ?? []).length > 0;
  const steps: TourStep[] = [
    {
      title: "Welcome to ReviewDesk",
      description: `One place to see which emails go out, in which language and region, and to change them without copying by hand. You are signed in as a ${role.label}. This takes a minute; close it any time.`,
    },
    {
      target: "board",
      title: "Boards",
      description: "A board is one email sequence. Switch between them here.",
    },
    {
      target: "views",
      title: "Views",
      description:
        "Timeline shows the way of a deal: stages along a trunk, each stage's emails branching off it. Cards, Matrix and Archive show the same emails in other ways. Click an email to open it, read its status and comment.",
    },
    {
      target: "my-tasks",
      title: "My tasks",
      description: myTasksText(user),
    },
  ];
  if (edits) {
    steps.push({
      target: "new",
      title: "New",
      description:
        user.role === "reviewer"
          ? "Add a slot (one email in every language) or a single email to the boards you work on."
          : "Add a slot, an email, or a whole new board. A new board reaches no CRM robot until its emails are pasted there, so you can try things freely.",
    });
  }
  steps.push({
    target: "account",
    title: "Help and this tour",
    description: `Help in this menu explains every task, with a part for the ${role.label}. Replay tour brings this back.`,
  });
  return steps;
}

function myTasksText(user: TourUser) {
  if (user.role === "super_admin") {
    return "What waits for you: mentions, open comments on your emails, and the ready emails to paste into the Bitrix24 robots.";
  }
  if (user.role === "admin" || (user.managed_boards ?? []).length > 0 || (user.languages ?? []).length > 0) {
    return "What waits for you: mentions, open comments and drafts on your emails, and translations to update after the English email changed.";
  }
  return "Where someone mentioned you or answered your comment.";
}
