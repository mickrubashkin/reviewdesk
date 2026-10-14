# ReviewDesk Case Study

## Problem

HTML email review often happens across screenshots, chat threads, issue trackers, and ad hoc approvals. Feedback loses the exact content context, approvals become hard to audit after edits, and final handoff to CRM or ESP implementation can depend on manual status checking.

ReviewDesk focuses on the review layer before messages are implemented in downstream marketing, lifecycle, CRM, or partner communication systems.

## Solution

ReviewDesk provides a shared review workspace for HTML email sequences. Reviewers can comment on specific content blocks, admins can edit template-backed fields without changing the original HTML, and approval state becomes explicit, auditable, and stale-aware after content changes.

AI review acts as a workflow assistant for email effectiveness inside the journey, not as a generic copy generator. It can evaluate the email's stage, subject, preheader, body text, send timing, and review metadata to surface risks around clarity, user intent, conversion friction, localization, compliance, and production readiness.

The product is intentionally not a full email builder or marketing automation platform. It concentrates on the part of the workflow where teams need review context, approval gates, version history, and production handoff.

## Product Workflow

The intended demo story is:

1. Open a review board with seeded email sequence data.
2. Inspect an email preview with review anchors and existing metadata.
3. Add or resolve comments with severity.
4. Edit template-backed copy or planning fields.
5. Show approvals becoming stale after content changes.
6. Re-approve required areas.
7. Export or inspect the production handoff package.

## Architecture

- Backend: Go HTTP API with Chi, pgx, direct SQL, PostgreSQL migrations, and server-rendered email variants.
- Frontend: React, Vite, Mantine, TanStack Query, and React Router.
- Auth: OTP login, session cookies, admin and super-admin roles, and local dev-login bypass guarded by environment variables.
- Review model: comments attach to `data-review-block` anchors and text ranges; edited text can normalize affected anchors to whole-block context.
- Editing model: original HTML is preserved; admins edit `data-edit-*` fields and the backend renders final HTML.
- Approval model: blocking comments and required area approvals gate review and production approval.
- AI review: shared journey-aware analysis supports cached results and first-run streaming over SSE/EventSource.

## Tradeoffs

- Direct SQL keeps the backend explicit and easy to inspect, while a future `sqlc` migration can add type safety around the highest-risk query paths.
- The app uses frontend polling for comments instead of WebSocket/SSE because the current workflow does not need a full realtime collaboration layer.
- ReviewDesk avoids becoming a full email builder; template-backed edits preserve source HTML and keep implementation handoff predictable.
- Engineering priorities focus on reproducible checks, demo datasets, and clear documentation before broader feature expansion.

## Result

ReviewDesk demonstrates a complete product-engineering slice: authenticated workflow software, PostgreSQL-backed domain state, review anchoring, approval gates, version-aware editing, AI-assisted analysis, and production handoff. The public demo uses disposable synthetic data so the workflow can be shown without exposing production content.
