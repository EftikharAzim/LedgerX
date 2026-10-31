# LedgerX Documentation

Start here for the design and reference docs. All diagrams are Mermaid and
render directly on GitHub.

| Doc | What's inside |
| --- | --- |
| [PRODUCT_PLAN.md](./PRODUCT_PLAN.md) | Personal money tracking direction, public research, customer validation, and a minimal delivery roadmap |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System context, component layering, runtime processes, request lifecycle, technology choices |
| [DATA_MODEL.md](./DATA_MODEL.md) | Entity-relationship diagram, the double-entry invariants, external account, append-only reversals, balance computation, migration history |
| [SEQUENCES.md](./SEQUENCES.md) | Sequence diagrams for every key flow — transaction creation, idempotency, transfers, reversals, balance reads, async export, the outbox publisher, and auth |
| [AI_CATEGORIZATION.md](./AI_CATEGORIZATION.md) | LLM transaction categorization — the suggest-then-confirm correctness boundary, the async flow, pluggable backends, and the evals harness |
| [API.md](./API.md) | HTTP endpoint reference |

Suggested reading order for understanding the codebase: **ARCHITECTURE → DATA_MODEL → SEQUENCES → API**.
