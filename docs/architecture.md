# Architecture

This page gives the C4 container view and one component zoom. Read
[design.md](design.md) for the reasoning and [drivers.md](drivers.md) for the
quality scenarios. The root [README](../README.md) has the system context in
prose.

## 1. System context

Noumena serves two actors: the creator (or an analyst in the creator's
workspace) and the operator. It depends on Google for OAuth and the YouTube
Data API, on one or two LLM providers, on Stripe for billing, and on an email
provider. The operator runs the binary and the database on one host.

```mermaid
flowchart LR
    Creator(["Creator / analyst<br/>[person]"])
    Operator(["Operator<br/>[person]"])
    Noumena["Noumena<br/>[software system]<br/>Comment ingestion, LLM insight, billing"]
    YouTube["YouTube Data API<br/>[external system]"]
    Google["Google OAuth<br/>[external system]"]
    LLM["LLM providers<br/>[external system]"]
    Stripe["Stripe<br/>[external system]"]
    Mail["SMTP or Resend<br/>[external system]"]

    Creator -->|"uses, HTTPS"| Noumena
    Operator -->|"operates, shell / HTTPS"| Noumena
    Noumena -->|"reads comments, HTTPS/JSON"| YouTube
    Noumena -->|"authorizes, HTTPS/OAuth 2.0"| Google
    Noumena -->|"classifies, HTTPS/JSON"| LLM
    Noumena -->|"charges, HTTPS/JSON"| Stripe
    Stripe -->|"webhooks, HTTPS/JSON"| Noumena
    Noumena -->|"sends, SMTP or HTTPS/JSON"| Mail
```

## 2. Container view

The API and the workers are one deployable: `cmd/api` starts the HTTP server
and the worker goroutines. They are separate containers here because they
have separate scaling and failure behaviour, not separate processes.

```mermaid
flowchart TB
    subgraph Client["Client"]
        SPA["Vue 3 SPA<br/>[container: TypeScript + Vite]<br/>Dashboard, feed, settings"]
    end

    subgraph Noumena["Noumena host"]
        API["HTTP API<br/>[container: Go, chi]<br/>Auth, workspace, feed, billing, lifecycle"]
        WRK["Background workers<br/>[container: Go goroutines]<br/>Scheduler, ingestion, LLM, digest, export, deletion"]
        DB[("SQLite<br/>[container: database]<br/>WAL, single writer")]
        FS[("Export volume<br/>[container: filesystem]<br/>CSV/JSON, GDPR ZIPs")]
    end

    subgraph External["External services"]
        YT["YouTube Data API"]
        GO["Google OAuth"]
        LLM["LLM providers"]
        ST["Stripe"]
        MAIL["SMTP / Resend"]
        OTEL["OTLP collector"]
    end

    SPA -->|"requests, HTTPS/JSON"| API
    API -->|"JSON responses, HTTPS"| SPA
    API -->|"reads and writes, SQL over file I/O"| DB
    WRK -->|"reads and writes, SQL over file I/O"| DB
    API -->|"writes exports, POSIX file I/O"| FS
    WRK -->|"writes exports, POSIX file I/O"| FS
    API -->|"token exchange, HTTPS/OAuth 2.0"| GO
    WRK -->|"comment fetch, HTTPS/JSON"| YT
    WRK -->|"classification, HTTPS/JSON"| LLM
    API -->|"checkout and portal, HTTPS/JSON"| ST
    ST -->|"webhooks, HTTPS/JSON"| API
    WRK -->|"transactional email, SMTP or HTTPS/JSON"| MAIL
    API -->|"traces and metrics, OTLP/gRPC"| OTEL
    WRK -->|"traces and metrics, OTLP/gRPC"| OTEL
```

Notes:

- The SPA is served by Vite in development and as static files in a
  deployment. The repository has no production static-file server.
- The OTLP exporter is optional. With `OTEL_ENABLED=false` every
  instrumentation call is a no-op.
- There is no Redis, no message broker, and no second database. The queue is a
  table (`ingestion_jobs`, `llm_processing_jobs`, `account_deletion_jobs`,
  `data_export_jobs`).

## 3. Component zoom: comment read path

Every comment surfaced to a user goes through `BuildFeedQuery`
(`internal/db/comments_feed.go`). The zoom shows why: one predicate protects
four modes and the export stream.

```mermaid
flowchart LR
    subgraph API["HTTP API"]
        MW["AuthMiddleware<br/>[component]"]
        WM["WithWorkspaceID + WorkspaceMiddleware<br/>[component]<br/>resolves workspace, loads role"]
        IH["InsightsHandler<br/>[component]<br/>Feed, Saved, Search, Thread, Export"]
        EXP["Export worker<br/>[component]<br/>streams via ModeStream"]
    end

    subgraph DB["internal/db"]
        BFQ["BuildFeedQuery<br/>[component]<br/>single SQL builder"]
        AC["accessControlClause<br/>[component]<br/>tracking OR workspace grant"]
        FTS["comments_fts<br/>[component]<br/>FTS5 search index"]
    end

    subgraph Store["SQLite"]
        PC[("platform_channels")]
        CM[("comments")]
        UCT[("user_channel_tracking")]
        GR[("channel_grants + workspace_members")]
    end

    MW -->|"user ID in context, in-process"| WM
    WM -->|"authorized request, in-process"| IH
    IH -->|"filters + mode, in-process call"| BFQ
    EXP -->|"filters + ModeStream, in-process call"| BFQ
    BFQ -->|"emits unconditionally, SQL text"| AC
    BFQ -->|"SELECT, SQL"| CM
    BFQ -->|"JOIN, SQL"| PC
    AC -->|"EXISTS, SQL"| UCT
    AC -->|"EXISTS, SQL"| GR
    BFQ -->|"MATCH via subquery, SQL"| FTS
```

The read path is intentionally boring: one builder, positional parameters, no
string interpolation of user input. `comments_feed_test.go` includes an
injection test and three grant tests. The CI job `fitness-tenant-isolation`
runs the isolation tests.

## 4. Deployment shape

One host, one process, one SQLite file, one export volume. The single-writer
rule is documented in `docs/ops/runbooks/incident.md`. A second API instance
would race on job claims and on the database file, so scale-out is a redesign,
not a config change. This is the main architectural risk, recorded in
[ADR-0005](adr/0005-sqlite-single-writer.md).
