import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef } from "react";

import { demoMode, finishTour } from "../auth/api";
import type { AuthUser } from "../emails/types";
import { shouldStartTour, TOUR_VERSION, tourSteps } from "./tour";

const replayEvent = "reviewdesk:replay-tour";

// replayTour opens the tour again on the board page.
export function replayTour() {
  window.dispatchEvent(new Event(replayEvent));
}

// firstVisible picks the shown one of the elements with this data-tour: the
// header keeps some controls twice, one for each width.
function firstVisible(target: string) {
  return (
    Array.from(document.querySelectorAll<HTMLElement>(`[data-tour="${target}"]`)).find(
      (element) => element.getClientRects().length > 0
    ) ?? null
  );
}

// useBoardTour opens the tour once the board header is on screen, for a
// person who has not finished or skipped this version yet, and again on
// replayTour. Closing it at any step counts as done.
export function useBoardTour(currentUser: AuthUser, ready: boolean) {
  const queryClient = useQueryClient();
  const started = useRef(false);

  const start = useCallback(async () => {
    // driver.js loads only when a tour opens, once per person.
    const [{ driver }] = await Promise.all([
      import("driver.js"),
      import("driver.js/dist/driver.css"),
      import("./tour.css"),
    ]);
    const steps = tourSteps(currentUser).flatMap((step) => {
      if (!step.target) {
        return [{ popover: { title: step.title, description: step.description } }];
      }
      const element = firstVisible(step.target);
      return element ? [{ element, popover: { title: step.title, description: step.description } }] : [];
    });
    const tour = driver({
      doneBtnText: "Done",
      nextBtnText: "Next",
      popoverClass: "reviewdesk-tour",
      prevBtnText: "Back",
      showProgress: true,
      steps,
      onDestroyed: () => {
        if ((currentUser.tour_version ?? 0) >= TOUR_VERSION) {
          return;
        }
        finishTour(TOUR_VERSION)
          .then(({ tour_version }) =>
            queryClient.setQueryData<AuthUser>(["auth", "me"], (user) => (user ? { ...user, tour_version } : user))
          )
          .catch(() => {
            // Not saved: the tour opens again next time, which is harmless.
          });
      },
    });
    tour.drive();
  }, [currentUser, queryClient]);

  useEffect(() => {
    if (ready && !started.current && shouldStartTour(currentUser, demoMode)) {
      started.current = true;
      void start();
    }
  }, [currentUser, ready, start]);

  useEffect(() => {
    const replay = () => void start();
    window.addEventListener(replayEvent, replay);
    return () => window.removeEventListener(replayEvent, replay);
  }, [start]);
}
