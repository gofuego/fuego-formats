---
title: Use PostgreSQL
status: Accepted
author: [alice, bob]
approvers: carol
tags: [database, infrastructure]
date_proposed: 2026-05-02
date_accepted: 2026-05-20
supersedes: 3
affects:
  - "internal/store/**"
  - "migrations/*.sql"
---
Adopted after the spike in sprint 12.

## Context

We need a relational database. This revisits [003](0003-use-sqlite.adr.md)
and its [decision](0003-use-sqlite.adr.md#decision). Background reading:
https://example.com/notes.adr.md.

## Decision

We will use **PostgreSQL** for all persistent storage.

## Consequences

- Strong ecosystem
- Operational overhead

## Options Considered

| Option | Verdict |
|---|---|
| SQLite | too small |
| PostgreSQL | chosen |
