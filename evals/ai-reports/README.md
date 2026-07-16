# AI Report Goldens

Status: **golden fixtures only**.

These cases document representative inputs and good `ai_report_output.v1`
responses for Yauli AI reports. They do not call OpenAI and are not wired into
CI yet.

The goal is to make expected behavior reviewable before building a fuller eval
runner.

The report should read like curated parent-facing insight, not a statistical
recap. A good output identifies the one or two strongest supported takeaways,
uses only the totals needed to support them, and omits low-value facts.

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
* verify highlights do not duplicate every deterministic total or simply repeat
  the daily summary;
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
* avoid invented facts, medical advice, diagnosis, treatment advice, urgency,
  or safety claims.

The first runner should be deterministic and local-only. Model-calling evals
can come later once prompt iteration starts.
