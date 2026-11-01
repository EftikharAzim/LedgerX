# LedgerX Documentation

Start here for the design and reference docs. All diagrams are Mermaid and
render directly on GitHub.

| Doc | What's inside |
| --- | --- |
| [PRODUCT_PLAN.md](./PRODUCT_PLAN.md) | Personal money tracking direction, public research, customer validation, and a minimal delivery roadmap |
| [LAUNCH_AND_SCALING.md](./LAUNCH_AND_SCALING.md) | Current public-access gaps, zero-spend launch options, scaling constraints, and a capacity-test plan |
| [CUSTOMER_REQUIREMENTS.md](./CUSTOMER_REQUIREMENTS.md) | Expanded research across real-life scenarios, evidence limits, requirements, and release priorities |
| [UI_UX_REVIEW.md](./UI_UX_REVIEW.md) | Distraction reduction, free personal category hints, UI/UX checks, and AI scope |
| [IMPORTS_AND_BALANCE_CHECKS.md](./IMPORTS_AND_BALANCE_CHECKS.md) | Guided CSV import, duplicate review, starting corrections, saved receipts, balance comparisons, and recovery |
| [WORKSPACE_RECOVERY.md](./WORKSPACE_RECOVERY.md) | Reviewed transaction editing, private JSON workspace backups, atomic restores, and recovery limits |
| [CUSTOMER_TEST_KIT.md](./CUSTOMER_TEST_KIT.md) | Six tasks for five customer sessions, dummy data, observations, and an evidence summary tool |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System context, component layering, runtime processes, request lifecycle, technology choices |
| [DATA_MODEL.md](./DATA_MODEL.md) | Entity-relationship diagram, the double-entry invariants, external account, append-only reversals, balance computation, migration history |
| [SEQUENCES.md](./SEQUENCES.md) | Sequence diagrams for every key flow — transaction creation, idempotency, transfers, reversals, balance reads, async export, the outbox publisher, and auth |
| [AI_CATEGORIZATION.md](./AI_CATEGORIZATION.md) | LLM transaction categorization — the suggest-then-confirm correctness boundary, the async flow, pluggable backends, and the evals harness |
| [API.md](./API.md) | HTTP endpoint reference |

Suggested reading order for understanding the codebase: **ARCHITECTURE → DATA_MODEL → SEQUENCES → API**.
