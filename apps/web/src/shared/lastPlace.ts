import { useEffect } from "react";
import { useLocation } from "react-router-dom";

// The last page a person was on, so that opening the app again (the portal
// loads its root each time) brings them back there instead of the default
// board. Kept in localStorage: it lives across visits, per browser.
const storageKey = "reviewdesk:last-place";

// Pages that make no sense to land on later: forms that start something new.
const skippedPaths = ["/emails/new", "/slots/new"];

export function placeWorthKeeping(pathname: string): boolean {
  return pathname !== "/" && !skippedPaths.includes(pathname);
}

// lastPlaceToRestore gives the stored place when the root was opened as the
// first page of this load, so that going to "/" later in a visit still lands
// on the default board.
export function lastPlaceToRestore(stored: string | null, firstPageOfLoad: boolean): string | null {
  if (!firstPageOfLoad || !stored || !stored.startsWith("/") || stored.startsWith("//")) {
    return null;
  }
  const pathname = stored.split(/[?#]/, 1)[0];
  return placeWorthKeeping(pathname) ? stored : null;
}

let firstPageOfLoad = true;

export function useRememberPlace() {
  const location = useLocation();

  useEffect(() => {
    if (!placeWorthKeeping(location.pathname)) {
      return;
    }
    firstPageOfLoad = false;
    try {
      window.localStorage.setItem(storageKey, location.pathname + location.search + location.hash);
    } catch {
      // Storage may be unavailable in restricted browser contexts.
    }
  }, [location.pathname, location.search, location.hash]);
}

export function readPlaceToRestore(): string | null {
  let stored: string | null;
  try {
    stored = window.localStorage.getItem(storageKey);
  } catch {
    stored = null;
  }
  return lastPlaceToRestore(stored, firstPageOfLoad);
}

// The root page calls this once it has decided where to go.
export function markRootVisited() {
  firstPageOfLoad = false;
}
