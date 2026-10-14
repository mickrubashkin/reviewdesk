# ReviewDesk Frontend Architecture

This document provides a high-level overview of the ReviewDesk frontend architecture. It is designed to help new team members quickly understand how the codebase is organized, what technologies are used, and where to find key components.

## Tech Stack

The frontend is a modern Single Page Application (SPA) built with:

- **Framework**: [React 19](https://react.dev/) + [Vite](https://vitejs.dev/)
- **Language**: TypeScript
- **UI Library**: [Mantine v9](https://mantine.dev/) (provides core components, hooks, and notifications)
- **Routing**: [React Router v7](https://reactrouter.com/)
- **Data Fetching / State Management**: [TanStack Query v5](https://tanstack.com/query/latest) (React Query)
- **Icons**: [Phosphor Icons](https://phosphoricons.com/)
- **CSS**: Vanilla CSS Modules (with PostCSS & Mantine presets)

## Directory Structure

The project follows a **Feature-based architecture** (inspired by Feature-Sliced Design). Code is grouped by domain/feature rather than by technical role (e.g., instead of having all components in `src/components`, they live inside their respective feature folders).

```text
apps/web/
├── src/
│   ├── features/               # Domain-specific modules
│   │   ├── admin-table/        # Reusable admin table UI components
│   │   ├── admin-users/        # User management screens
│   │   ├── ai-logs/            # AI analysis event logs
│   │   ├── auth/               # Login components
│   │   ├── auth-events/        # Auth audit logs
│   │   ├── boards/             # Kanban boards for email states
│   │   ├── email-events/       # Email audit logs
│   │   ├── emails/             # Core email entities and god-API module
│   │   ├── ops-events/         # Operational events
│   │   └── review/             # Email Review screen (core product)
│   ├── App.tsx                 # Main application router
│   ├── main.tsx                # React entry point & Provider setup
│   ├── theme.ts                # Mantine theme configuration
│   └── vite-env.d.ts           # Vite types
```

## Data Fetching & API

We do not use Redux, Zustand, or Context for global API state. All remote data fetching and mutations are handled by **TanStack Query**. 

Currently, all API calls are centralized in a single file: `features/emails/api.ts`. This file exposes pure async functions that hit the backend via `fetch()`. Components wrap these functions in `useQuery` or `useMutation`.

> [!WARNING]
> `api.ts` is currently acting as a "god module" and contains endpoints for auth, boards, admin users, etc. This is a known technical debt that needs refactoring.

### AI Streaming (SSE)
For real-time AI analysis streaming, we use Server-Sent Events (SSE). The `useEmailAnalysisStream` hook consumes the `/api/emails/{id}/ai-analysis-stream` endpoint using standard `EventSource`. We **do not** use WebSockets.

## Core Features

### EmailReview (The Review Screen)
The most complex part of the app is the `EmailReview` feature. It was recently refactored to prevent becoming a monolithic component. 

- **State & Logic**: Extracted into custom hooks (`useEmailReviewData`, `useEmailReviewMutations`, `useEmailReviewLayout`).
- **UI Sections**: Split into semantic directories:
  - `header/`: Controls for status, viewport variants.
  - `panels/`: Right-side tabs (Activity, Approvals, Handoff, History, Planning).
  - `comments/`: Comment rendering, composing, and filtering.
- **Preview Isolation**: Third-party HTML emails are rendered inside an isolated `iframe` (Sandbox) within `MailPreview.tsx` to prevent XSS and style bleed.

### Boards (The Kanban View)
The `BoardHome` feature serves as the dashboard. It fetches columns (stages) and cards (emails) using the unified `useBoardHomeData` hook. 

## Routing Strategy
Routing is handled in `App.tsx` using `Routes` and `Route`. We have an `AuthenticatedApp` wrapper that checks session state via `fetchCurrentUser`. 
- `/boards/:boardKey` — Main Kanban view.
- `/emails/:emailId/review` — Email Review screen.
- `/admin/*` — Admin panel screens.

Unauthenticated users are dropped into the `<Login />` component.

## Best Practices & Guidelines
1. **No direct DOM manipulation**: Stick to React state.
2. **Use Mantine**: Don't build custom buttons, inputs, or modals if Mantine already provides them.
3. **Vanilla CSS**: We use standard CSS modules (`.module.css`). We do not use TailwindCSS.
4. **Thin components**: If a component exceeds 300 lines, consider extracting its logic into a custom hook or splitting its UI into smaller sub-components.
