# ReviewDesk Roadmap

ReviewDesk focuses on an explicit review and approval workflow for staged HTML email sequences rather than open-ended authoring.

## Milestones

- [x] Disposable synthetic demo dataset (`demo-onboarding` board with EN masters and linked translations, comments, Draft / Ready statuses, a translation behind its master, and handoff examples), copied into a sandbox per visitor.
- [x] Continuous integration checks: top-level `make lint`, `make test-api`, `make test-web`, and GitHub Actions.
- [x] Draft / Ready review status with checks of form before Ready; an edit moves a Ready email back to Draft. Area approvals and the blocking-comment gate were removed on 2026-11-01.
- [ ] Safety controls: confirmation before deleting board stages.
- [ ] Centralized authentication lifecycle handling (session-expired recovery and reconnects).
- [ ] AI architecture enhancements: structured AI logs, prompt versioning, and evaluation fixtures.
- [ ] Type-safe query migration: introduce `sqlc` for the highest-risk query paths (status changes, versions, portal sync).

## In Scope

- Review context attached to HTML email content.
- An explicit Draft / Ready status that follows the content: checks of form before Ready, back to Draft after an edit.
- Production handoff for Ready emails and tracking of what the CRM actually sends.
- AI-assisted review of email effectiveness inside the journey.
- Bounded, high-impact backend engineering improvements.

## Out Of Scope

- Full email builder / WYSIWYG canvas.
- Multi-tenant workspaces and billing.
- Direct CRM/ESP publishing integrations.
- MJML authoring.
- Broad permission-model redesign.
- Full visual redesign.
