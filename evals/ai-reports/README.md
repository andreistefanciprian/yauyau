# AI Report Goldens

Status: **golden fixtures only**.

These cases document representative inputs and good `ai_report_output.v1`
responses for Yauli AI reports. They do not call OpenAI and are not wired into
CI yet.

The goal is to make expected behavior reviewable before building a fuller eval
runner.

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
* check required phrases and forbidden terms;
* verify partial reports use partial wording;
* verify absent comparison data does not produce comparison claims;
* verify parent notes are attributed as notes;
* avoid medical advice, diagnosis, urgency, or safety claims.

The first runner should be deterministic and local-only. Model-calling evals
can come later once prompt iteration starts.
