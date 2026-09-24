# Chronicle — UX Flows & Architecture Diagrams

All diagrams are Mermaid syntax. Render with any Mermaid-compatible viewer (GitHub, VS Code, mermaid.live).

---

## 1. Navigation Map — All Routes & Transitions

```mermaid
graph TD
    A[Dashboard<br/>⌘1] --> B[Investigation Workbench<br/>⌘2]
    A --> C[Timeline Explorer<br/>⌘3]
    A --> D[Entity Graph<br/>⌘4]
    A --> E[Sessions<br/>⌘5]
    A --> F[Memory Browser<br/>⌘6]
    A --> G[Task Queue<br/>⌘7]
    A --> H[Approvals<br/>⌘8]
    A --> I[Billing<br/>⌘9]
    A --> J[System Health<br/>⌘0]
    A --> K[Admin<br/>⌘-]
    A --> L[Settings<br/>⌘=]
    
    B --> B1[THINK Pane]
    B --> B2[SAYS Pane]
    B --> B3[Evidence Panel<br/>Ctrl+E]
    B --> B4[Discovery Panel<br/>Ctrl+Shift+D]
    B --> B5[Investigation Switcher<br/>Ctrl+Shift+I]
    
    E --> E1[Session Detail]
    E1 --> B
    
    C --> B
    D --> B
    
    F --> F1[Memory Event Detail]
    F1 --> B
    
    style A fill:#388bfd,stroke:#58a6ff,color:#fff
    style B fill:#a371f7,stroke:#c084fc,color:#fff
```

---

## 2. Investigation Workbench — THINK/SAYS Interaction Flow

```mermaid
sequenceDiagram
    actor User
    participant Input as Input Area
    participant API as Consensus API
    participant WS as WebSocket
    participant THINK as THINK Pane
    participant SAYS as SAYS Pane
    participant Evidence as Evidence Panel

    User->>Input: Types query + Enter
    Input->>Input: Collapses to "Processing..."
    Input->>API: POST /sessions/:id/message
    API-->>WS: iteration.started event
    
    WS->>THINK: Create Thought Card (THINKING state)
    THINK->>THINK: Streaming cursor appears ▊
    
    loop AI Reasoning
        WS->>THINK: memory.created (content chunk)
        THINK->>THINK: Append text at 60fps
    end
    
    WS->>THINK: memory.created (is_final: true)
    THINK->>THINK: Card → COMPLETED state
    THINK->>THINK: Cursor removed, timestamp appears
    
    WS->>SAYS: finding.created
    SAYS->>SAYS: Finding Card appears (DRAFT state)
    SAYS->>SAYS: Links to THINK Step N bidirectionally
    
    Input->>Input: Returns to IDLE state
    Input->>User: Auto-focus for next query

    User->>SAYS: Reviews Finding
    User->>SAYS: Clicks "Based on: Step 2"
    SAYS->>THINK: Scroll to Step 2 + highlight
    
    User->>SAYS: Clicks [Approve]
    SAYS->>API: PATCH /approvals/:id
    API-->>SAYS: approval.resolved
    SAYS->>SAYS: Green sweep animation (400ms)
    SAYS->>SAYS: Card → APPROVED state
    
    User->>Evidence: Drags source file
    Evidence->>Input: Adds @context pill
```

---

## 3. Session Lifecycle State Machine

```mermaid
stateDiagram-v2
    [*] --> booting: POST /sessions
    booting --> idle: Initialization complete
    idle --> thinking: User sends message
    thinking --> tool_exec: AI needs tool execution
    tool_exec --> thinking: Tool result received
    thinking --> waiting_sub: Delegated to sub-agent
    waiting_sub --> thinking: Sub-agent result
    thinking --> idle: Iteration complete, awaiting input
    idle --> paused: User pauses
    paused --> idle: User resumes
    thinking --> completed: Goal achieved / max iterations
    tool_exec --> completed: Goal achieved
    idle --> failed: Unrecoverable error
    thinking --> failed: Error threshold exceeded
    tool_exec --> failed: Error threshold exceeded
    thinking --> stalled: Heartbeat missed >90s
    stalled --> thinking: Heartbeat restored
    paused --> cancelled: User cancels
    idle --> cancelled: User cancels
    completed --> [*]
    failed --> [*]
    cancelled --> [*]

    note right of thinking
        Status dot: purple, pulsing
        Activity feed: "Iteration N started"
        Cost counter: actively incrementing
    end note

    note right of completed
        Status dot: green
        Row animation: green sweep left→right
        Toast: "Session completed"
    end note
```

---

## 4. Finding Approval State Machine

```mermaid
stateDiagram-v2
    [*] --> draft: AI generates finding
    
    draft --> approved: Human clicks Approve
    draft --> rejected: Human clicks Deny
    draft --> draft: Human requests revision
    
    approved --> draft: Human edits approved finding
    approved --> draft: Human clicks Unapprove
    
    rejected --> draft: Human clicks Reconsider
    rejected --> [*]: (hidden from default view)

    draft --> outdated: Newer finding supersedes
    approved --> outdated: Newer finding supersedes

    note right of draft
        Border: 1px border-default
        Badge: "Draft" pill, text-secondary
        Actions: [Approve] [Revise] [Edit] [Flag]
        Animation: none (static)
    end note

    note right of approved
        Border-left: 3px accent-success
        Badge: "✓ Approved" green pill
        Actions: [Edit] [Unapprove] [Copy]
        Animation: green sweep 400ms on approve
        Audit: approver name + timestamp recorded
    end note

    note right of rejected
        Border-left: 3px accent-error
        Opacity: 60%
        Badge: "✗ Rejected" red pill
        Actions: [Reconsider] [Copy]
        Reason: optional text shown in italics
    end note
```

---

## 5. Data Flow Architecture — API + WebSocket + State

```mermaid
graph TD
    subgraph "Browser"
        UI[React SPA]
        RQ[React Query Cache]
        ZS[Zustand UI State]
        LS[localStorage]
    end

    subgraph "Consensus Server"
        API[REST API<br/>:8090]
        WS[WebSocket<br/>:8090/ws]
        DB[(SQLite/Postgres)]
        AGENT[Agent Runtime]
    end

    subgraph "External"
        LLM[LLM Provider<br/>DeepSeek/Claude]
    end

    UI -->|Queries| RQ
    RQ -->|fetch| API
    API --> DB
    UI -->|Mutations| API
    API -->|Invalidates| RQ
    
    WS -->|Events| RQ
    RQ -->|Re-render| UI
    
    UI -->|Persist prefs| LS
    LS -->|Restore| ZS
    
    AGENT -->|Calls| LLM
    LLM -->|Response| AGENT
    AGENT --> DB
    AGENT -->|Broadcasts| WS
    
    WS -->|session.status| RQ
    WS -->|memory.created| RQ
    WS -->|iteration.started| RQ
    WS -->|finding.created| RQ
    WS -->|approval.requested| RQ
    WS -->|billing.updated| RQ

    style UI fill:#a371f7,stroke:#c084fc,color:#fff
    style API fill:#388bfd,stroke:#58a6ff,color:#fff
    style WS fill:#39d2c0,stroke:#22d3ee,color:#0d1117
    style DB fill:#238636,stroke:#3fb950,color:#fff
    style AGENT fill:#d29922,stroke:#ffa657,color:#0d1117
    style LLM fill:#da3633,stroke:#f85149,color:#fff
```

---

## 6. WebSocket Event Flow

```mermaid
sequenceDiagram
    participant Client as Browser
    participant WS as WebSocket Server
    participant Agent as Agent Runtime
    participant DB as Database

    Client->>WS: Connect (ws://host:8090/ws)
    WS-->>Client: Connected
    
    Client->>WS: subscribe channels: ['session:a3f', 'system']
    WS-->>Client: Subscribed

    Agent->>DB: Write memory event
    Agent->>WS: memory.created {session_id, type, content_chunk, is_final}
    WS->>Client: memory.created

    Agent->>DB: Update session status
    Agent->>WS: session.status {id, status: 'thinking', previous: 'idle'}
    WS->>Client: session.status

    Agent->>LLM: API call
    LLM-->>Agent: Response
    Agent->>WS: iteration.started {session_id, iteration_number}
    WS->>Client: iteration.started

    Agent->>DB: Write finding
    Agent->>WS: finding.created {id, session_id, title, confidence}
    WS->>Client: finding.created

    Agent->>DB: Update billing
    Agent->>WS: billing.updated {session_id, tokens_used, cost}
    WS->>Client: billing.updated

    Client->>WS: ping (every 30s)
    WS-->>Client: pong

    Note over Client,WS: If no pong in 45s: reconnect<br/>Exponential backoff: 1s,2s,4s,8s,15s max
```

---

## 7. Command Palette — Search Flow

```mermaid
flowchart TD
    A[User presses Ctrl+K] --> B{Input empty?}
    B -->|Yes| C[Show Recent Items<br/>5 most recent pages/sessions]
    B -->|No| D[Debounce 100ms]
    D --> E{Prefix?}
    E -->|">" prefix| F[Command Mode<br/>Search commands only]
    E -->|"#" prefix| G[Session Mode<br/>Search sessions only]
    E -->|No prefix| H[Fuzzy Match All<br/>Pages + Sessions + Commands]
    
    F --> I[Score: prefix > word-boundary > substring > fuzzy]
    G --> I
    H --> I
    
    I --> J[Display Results<br/>Grouped by section]
    J --> K{User action?}
    K -->|Arrow keys| L[Move selection]
    K -->|Enter| M[Execute selected]
    K -->|Escape| N[Close palette]
    K -->|Type more| D
    
    L --> J
    M --> O[Close palette + navigate/execute]
    N --> P[Return focus to trigger element]

    style A fill:#a371f7,stroke:#c084fc,color:#fff
    style C fill:#1c2128,stroke:#484f58,color:#e6edf3
    style J fill:#161b22,stroke:#388bfd,color:#e6edf3
```

---

## 8. Component Hierarchy

```mermaid
graph TD
    App[App Shell]
    App --> TB[Top Bar]
    App --> SB[Sidebar]
    App --> Content[Content Area<br/>React Router Outlet]
    App --> StB[Status Bar]
    
    TB --> Logo[Logo]
    TB --> CP[Command Palette Trigger]
    TB --> Notif[Notification Bell]
    TB --> Profile[Profile Avatar]
    
    SB --> NavItems[Navigation Items<br/>10 routes]
    SB --> Collapse[Collapse Toggle]
    
    Content --> Dashboard[Dashboard Page]
    Content --> Workbench[Investigation Workbench]
    Content --> Timeline[Timeline Explorer]
    Content --> Graph[Entity Graph]
    Content --> Sessions[Sessions Page]
    Content --> Memory[Memory Browser]
    Content --> Tasks[Task Queue]
    Content --> Approvals[Approvals Page]
    Content --> Billing[Billing Page]
    Content --> Health[System Health]
    Content --> Admin[Admin Panel]
    Content --> Settings[Settings Page]
    
    Dashboard --> KPI[KPI Bar<br/>6 KPI Cards]
    Dashboard --> Feed[Activity Feed]
    Dashboard --> Recent[Recent Sessions]
    Dashboard --> Charts[Status Donut + Model Usage]
    
    Workbench --> ThinkPane[THINK Pane]
    Workbench --> Divider[Resizable Divider]
    Workbench --> SaysPane[SAYS Pane]
    Workbench --> Evidence[Evidence Panel]
    Workbench --> InputArea[Input Area]
    
    ThinkPane --> ThoughtCard[Thought Card<br/>× N per session]
    SaysPane --> FindingCard[Finding Card<br/>× N per session]
    
    ThoughtCard --> SourceBadge[Source Badge]
    ThoughtCard --> FlagIndicator[Flag Indicator]
    FindingCard --> ConfidenceBar[Confidence Bar]
    FindingCard --> ApprovalWorkflow[Approval Workflow]
    
    style App fill:#0d1117,stroke:#484f58,color:#e6edf3
    style Workbench fill:#a371f7,stroke:#c084fc,color:#fff
    style ThinkPane fill:#1c2128,stroke:#a371f7,color:#e6edf3
    style SaysPane fill:#1c2128,stroke:#238636,color:#e6edf3
```

---

## 9. Streaming Rendering Pipeline

```mermaid
flowchart LR
    WS[WebSocket<br/>memory.created] --> Queue[Chunk Queue]
    Queue --> RAF[requestAnimationFrame<br/>throttled 16ms]
    RAF --> Batch[Batch all chunks<br/>since last RAF]
    Batch --> Card{Thought Card<br/>exists for iteration?}
    Card -->|No| Create[Create new Card<br/>THINKING state]
    Card -->|Yes| Append[Append text to Card]
    Create --> Append
    Append --> Cursor[Update cursor position<br/>▊ blink 1s cycle]
    Cursor --> Scroll[Auto-scroll<br/>keep cursor visible]
    
    WS2[WebSocket<br/>is_final: true] --> Remove[Remove cursor span]
    Remove --> Complete[Card → COMPLETED state]
    Complete --> Timestamp[Timestamp fades in<br/>200ms]
    
    style WS fill:#39d2c0,stroke:#22d3ee,color:#0d1117
    style RAF fill:#d29922,stroke:#ffa657,color:#0d1117
    style Create fill:#a371f7,stroke:#c084fc,color:#fff
    style Complete fill:#238636,stroke:#3fb950,color:#fff
```

---

## 10. Approval Workflow — Full Sequence

```mermaid
sequenceDiagram
    actor Analyst
    participant SAYS as SAYS Pane
    participant API as REST API
    participant DB as Database
    participant Audit as Audit Trail
    participant Agent as AI Agent

    Note over SAYS: Finding #7 in DRAFT state

    Analyst->>SAYS: Reviews finding content
    Analyst->>SAYS: Clicks [Approve]
    
    SAYS->>SAYS: Show confirmation dialog
    Analyst->>SAYS: Confirms + adds note
    
    SAYS->>API: PATCH /approvals/:id<br/>{status: 'approved', note: '...'}
    API->>DB: UPDATE approvals SET status='approved'
    API->>Audit: INSERT approval event + hash
    API-->>SAYS: 200 OK
    
    SAYS->>SAYS: Green sweep animation (400ms)
    SAYS->>SAYS: Card → APPROVED state
    SAYS->>SAYS: Badge: "✓ Approved by Bane · just now"
    
    API-->>Agent: approval.resolved event
    Agent->>Agent: Continue with next step

    Note over SAYS: Finding #7 in APPROVED state

    opt Request Revision
        Analyst->>SAYS: Clicks [Request Revision]
        SAYS->>SAYS: Revision dialog
        Analyst->>SAYS: "Include Q3 comparison data"
        SAYS->>API: POST /sessions/:id/message<br/>{content: "Revise Finding #7: ..."}
        API-->>Agent: New message
        Agent->>Agent: Re-analyze with revision context
        Agent-->>SAYS: New draft: "Finding #7 (Revised)"
        SAYS->>SAYS: Revision history: v1 → v2
    end

    opt Unapprove
        Analyst->>SAYS: Clicks [Unapprove]
        SAYS->>SAYS: Confirmation: "Return to draft?"
        Analyst->>SAYS: Confirms
        SAYS->>API: PATCH /approvals/:id<br/>{status: 'draft'}
        SAYS->>SAYS: Card → DRAFT state
    end
```

---

## 11. Error Recovery Flow

```mermaid
stateDiagram-v2
    [*] --> Normal: System running

    state Normal {
        [*] --> Connected
        Connected --> Streaming: AI processing
        Streaming --> Connected: Iteration complete
    }

    state Error {
        WSDisconnect: WebSocket Lost
        APITimeout: API Timeout
        LLMError: LLM Provider Error
        DBError: Database Error
    }

    Connected --> WSDisconnect: Connection lost
    WSDisconnect --> Reconnecting: Auto-retry (backoff)
    Reconnecting --> Connected: Connection restored
    Reconnecting --> PersistentError: Max retries (5)
    PersistentError --> ManualRetry: User clicks [Retry]

    Streaming --> APITimeout: Request >30s
    APITimeout --> PartialSave: Save partial results
    PartialSave --> Connected: User retries or dismisses

    Streaming --> LLMError: Provider returns error
    LLMError --> Fallback: Try alternate model
    Fallback --> Streaming: Fallback accepted
    Fallback --> Connected: All providers failed

    Connected --> DBError: Database connection lost
    DBError --> ReadOnly: Switch to read-only mode
    ReadOnly --> Connected: DB connection restored

    note right of PersistentError
        Banner: "Connection lost — retrying..."
        Status bar: ● Disconnected (red, flashing)
        Partial data: cached via React Query
    end note
```

---

*Render these diagrams in VS Code (with Mermaid extension), GitHub (native support), or https://mermaid.live/*
