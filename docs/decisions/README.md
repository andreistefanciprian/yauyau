# Architectural Decisions

Important architectural choices should be recorded here using lightweight
Architecture Decision Records, one file per decision.

Recommended format:

```text
# Title

## Context
What problem or constraint led to the decision?

## Decision
What was chosen?

## Alternatives Considered
What credible alternatives were rejected?

## Consequences
What becomes easier, harder, or constrained?
```

Create or update a decision record when changing:

* service ownership boundaries;
* authentication or session architecture;
* event storage strategy;
* family membership rules;
* public API contracts;
* deployment topology;
* major dependencies or frameworks.

Do not reverse an existing architectural decision accidentally. Read the
relevant decision record first.
