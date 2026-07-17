# AI Daily Card Goldens

These fixtures document the dedicated `daily_card_input.v1` to
`daily_card_output.v1` behavior. They are intentionally separate from the
generic AI report goldens used by scheduled email and range reports.

The tests do not call OpenAI. They run the same deterministic validation used
before generated card prose is cached or returned to the frontend.

## Coverage

* a busy partial day with nappies, pumping, a bath, and a temperature check;
* every supplied current-day growth value must be included accurately in a warm, encouraging story;
* a sparse early day with no secondary-event story;
* one baby-name mention;
* relationship-aware encouragement;
* no feed or sleep KPI repetition;
* no nappy counts or subtype details;
* no Markdown, category icons, medical interpretation, or dash punctuation;
* at most one emoji, only in observation or encouragement.

## Run

```bash
cd backend-api
go test ./internal/handlers -run TestDailyCardGoldenFixtures
```

Model-calling and rubric-based evals can be added later if prompt iteration
needs quality measurement beyond the deterministic contract checks.
