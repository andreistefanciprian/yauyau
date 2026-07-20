# AI Daily Card Goldens

These fixtures document the dedicated `daily_card_input.v1` to
`daily_card_output.v2` behavior. They are intentionally separate from the
generic AI report goldens used by scheduled email and range reports.

The tests do not call OpenAI. They protect the structured contract and keep
reviewable examples of factual output. Factual faithfulness, naturalness,
warmth, and language variety belong in model-calling or rubric-based evals
rather than attempts to parse creative prose with hard-coded phrases.

## Coverage

* a busy partial day with nappies, pumping, a bath, and a temperature check;
* every supplied current-day growth value must be included accurately in a warm, encouraging story;
* a sparse early day with a modest factual body;
* a short generated title, normally using the baby name once;
* a brief closing that may use a parent-facing relationship;
* no feed or sleep KPI repetition;
* general nappy wording without counts or subtype details;
* exact pumping counts;
* supplied pumping volume is preserved exactly;
* useful comparisons are preferred over generic time-of-day commentary;
* Australian English flavour is optional, natural, varied, and never uses more than one expression or stereotypical slang;
* no Markdown, category icons, medical interpretation, or dash punctuation;
* at most one emoji, only in the body or closing.

## Run

```bash
cd backend-api
go test ./internal/handlers -run TestDailyCardGoldenFixtures
```

Model-calling and rubric-based evals can be added later if prompt iteration
needs quality measurement beyond the deterministic contract checks.
