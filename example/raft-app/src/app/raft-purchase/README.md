# purchase platform (mini)

## States

### Invoice

1. Published
2. Paid
3. Expired
4. Invalid

```mermaid
stateDiagram-v2
    [*] --> PUBLISHED(0): when seller initiate
    PUBLISHED(0) --> EXPIRED(3): when system detected expiry
    PUBLISHED(0) --> PAID(1): when buyer PAID (via 3rd party)
    PUBLISHED(0) --> INVALID(2): when seller marked it as INVALID

    PAID(1) --> [*]

    INVALID(2) --> [*]

    EXPIRED(3) --> [*]
```

