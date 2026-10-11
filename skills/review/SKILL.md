---
name: review
description: Use when Gautam wants to go through unverified or proposed rows ("review my queue", "what's waiting for approval", "approve those"), including after the 20:00 review nudge.
---

# Review

Walk the unverified rows one at a time. Gautam decides each one; you carry out the decision.

## Tools

`[planned]` is in the design doc but not built. `[missing]` is not in the design doc; the step that needs it cannot run yet.

| Tool | Status | Used for |
| --- | --- | --- |
| `review_queue` | [planned] | List unverified rows |
| `get` | [planned] | Read the source a row came from |
| `verify` | [planned] | Approve a row |
| `update` | [planned] | Fix a row before approving it |
| `delete` | [planned] | Remove a wrong row, after confirmation |
| lint findings in the queue | [missing] | Show and close findings filed by `lint` |

## Steps

1. **Count.** Call `review_queue`. Say how many rows are waiting, by type.
2. **Show one row.** Its type, its values, and where it came from: one line from the source row (`get` on `source_ref`), with the source ID.
3. **Take the decision.**

   | Gautam says | Do |
   | --- | --- |
   | Approve | `verify` |
   | Fix ("amount was 450") | `update` with his value, then `verify` |
   | Wrong, remove it | Confirm, then `delete` |
   | Skip | Leave it unverified |

4. **Next row.** Repeat until the queue is empty or he stops.
5. **Summarise.** Approved, fixed, deleted and skipped counts, and how many remain.

## Rules

- Never verify a row Gautam has not seen in this conversation.
- Batch approval happens only when he asks for it ("approve all the transactions"). List the rows in the batch first, then verify them.
- A fix uses his value exactly. If the service rejects it, relay the error and ask.
- Do not argue for or against a row. If the source contradicts it, show the source line and let him decide.
- Deleting is a soft delete; permanent removal happens only in the admin pages.
