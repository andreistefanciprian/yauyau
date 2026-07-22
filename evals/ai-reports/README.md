# AI Report Goldens

Status: **golden fixtures only**.

These cases document representative inputs and good `ai_report_output.v1`
responses for Yauli range and scheduled-email reports. They do not call OpenAI
and are not wired into CI yet. The daily KPI card and last-seven-days email
charts are deterministic and are covered by Go tests rather than model evals.

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

Each golden contains:

* `input`: the model-facing input envelope.
* `golden_output`: an example good structured response.
* `checks`: deterministic checks a future runner should enforce.

## Future Runner

A future eval runner should:

* validate `golden_output` as `ai_report_output.v1`;
* enforce array limits from the contract;
* verify `summary` is 1-2 short sentences and does more than enumerate event
  types;
* verify daily summaries and highlights do not repeat headline values already
  shown in the deterministic KPI card or last-seven-days charts;
* verify daily prose does not narrate day-by-day chart values or obvious chart
  shapes;
* verify outputs do not fill every section to its maximum length by default;
* verify the same insight is not repeated across summary, highlights, patterns,
  and comparison unless the repeated section adds new parent-facing value;
* verify nappy subtype breakdowns are omitted unless unusual or relevant to a
  parent-facing takeaway;
* verify durations are parent-friendly, such as "about 2 hours 20 minutes",
  rather than raw minute recaps;
* verify daily baseline grammar is natural without repeating the selected-day
  KPI, for example "Feeding frequency was above its recent daily average of
  8.9";
* check required facts and forbidden terms without requiring stock prose;
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
* use rubric-based review to confirm that any Australian English expression is
  natural, varied, optional, and limited to one across the report;
* reject explanations of expressions, English-lesson sections, and
  stereotypical slang such as "fair dinkum", "she'll be right", and "bonza";
* avoid invented facts, medical advice, diagnosis, treatment advice, urgency,
  or safety claims.
The first runner should be deterministic and local-only. Model-calling evals
can come later once prompt iteration starts.
