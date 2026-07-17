# AI Report Goldens

Status: **golden fixtures with deterministic contract validation**.

These cases document representative inputs and good `ai_report_output.v2`
responses for Yauli AI reports. They do not call OpenAI. The handler test suite
loads every fixture and applies the same structural and daily card validation
used before generated output is cached.

The goal is to make expected behavior reviewable before building a fuller eval
runner.

The report should read like curated parent-facing insight, not a statistical
recap. A good output identifies the one or two strongest supported takeaways,
uses only the totals needed to support them, and omits low-value facts. Empty
arrays are acceptable when a section has nothing useful to add.

## What These Goldens Cover

* Complete daily report.
* Today-so-far partial report.
* Weekly report.
* Notes-heavy report with no comparison.
* Relationship-aware daily card copy.
* Growth measurement coverage in the daily story.

Each golden contains:

* `input`: the model-facing input envelope.
* `golden_output`: an example good structured response.
* `checks`: deterministic checks a future runner should enforce.

## Run

```bash
cd backend-api
go test ./internal/handlers -run TestAIReportGoldenFixtures
```

## Checks

The deterministic validator and documented quality checks should:

* validate `golden_output` as `ai_report_output.v2`;
* enforce array limits from the contract;
* verify `summary` is 1-2 short sentences and does more than enumerate event
  types;
* verify highlights do not duplicate every deterministic total or simply repeat
  the daily summary;
* verify outputs do not fill every section to its maximum length by default;
* verify the same insight is not repeated across summary, highlights, patterns,
  and comparison unless the repeated section adds new parent-facing value;
* verify nappy subtype breakdowns are omitted unless unusual or relevant to a
  parent-facing takeaway;
* verify durations are parent-friendly, such as "about 2 hours 20 minutes",
  rather than raw minute recaps;
* verify baseline grammar is natural, for example "Seven feeds were logged,
  compared with a recent daily average of 8.9";
* check required phrases and forbidden terms;
* verify partial reports use partial wording;
* verify partial reports use "so far" wording for comparisons and do not
  present deltas as final daily outcomes;
* verify absent comparison data does not produce comparison claims;
* verify the comparison array is empty when backend comparison data is absent;
* verify parent notes are attributed as notes;
* verify pumping is not described as baby feeding or baby activity;
* verify relationship wording describes sequence only, not causation;
* verify rendered reports end with a short, supportive, non-medical
  encouragement for the parent;
* avoid invented facts, medical advice, diagnosis, treatment advice, urgency,
  or safety claims.
* require a recorded growth measurement to appear in the daily card story;
* keep nappy wording general without counts or subtype details;
* use the supplied relationship exactly once in encouragement;
* use the baby name exactly once in daily card prose;
* reject hyphen, en dash, and em dash punctuation in daily card prose, while
  preserving those characters inside supplied names or relationships;
* allow at most one emoji, only in observation or encouragement;
* keep feed and sleep metrics outside model prose so the UI can render the
  deterministic values with the only bold emphasis.

Model-calling and rubric-based evals can be added later if prompt iteration
needs quality measurement beyond deterministic contract checks.
