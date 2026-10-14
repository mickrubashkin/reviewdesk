# ReviewDesk Roadmap

ReviewDesk focuses on an explicit review and approval workflow for staged HTML email sequences rather than open-ended authoring.

## Milestones

- [x] Disposable synthetic demo dataset (`demo-onboarding` board with emails, comments, approval gates, stale states, and handoff packages).
- [x] Continuous integration checks: top-level `make lint`, `make test-api`, `make test-web`, and GitHub Actions.
- [ ] Safety controls: confirmation before deleting board stages or archiving approval areas.
- [ ] Centralized authentication lifecycle handling (session-expired recovery and reconnects).
- [ ] AI architecture enhancements: structured AI logs, prompt versioning, and evaluation fixtures.
- [ ] Type-safe query migration: introduce `sqlc` for high-integrity workflows (approval gates and area approvals).

## In Scope

- Review context attached to HTML email content.
- Explicit approvals and stale approval behavior after edits.
- Production handoff for approved assets.
- AI-assisted review of email effectiveness inside the journey.
- Bounded, high-impact backend engineering improvements.

## Out Of Scope

- Full email builder / WYSIWYG canvas.
- Multi-tenant workspaces and billing.
- Direct CRM/ESP publishing integrations.
- MJML authoring.
- Broad permission-model redesign.
- Full visual redesign.
