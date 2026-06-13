# LedgerX Documentation

Start here for the design and reference docs. All diagrams are Mermaid and
render directly on GitHub.

| Doc | What's inside |
| --- | --- |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System context, component layering, runtime processes, request lifecycle, technology choices |
| [DATA_MODEL.md](./DATA_MODEL.md) | Entity-relationship diagram, the double-entry invariants, external account, append-only reversals, balance computation, migration history |
| [SEQUENCES.md](./SEQUENCES.md) | Sequence diagrams for every key flow — transaction creation, idempotency, transfers, reversals, balance reads, async export, the outbox publisher, and auth |
| [API.md](./API.md) | HTTP endpoint reference |

Suggested reading order for understanding the codebase: **ARCHITECTURE → DATA_MODEL → SEQUENCES → API**.
