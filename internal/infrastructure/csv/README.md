# CSV import/export format

The fixed CSV model shared by `POST /import/movements` (parse, BACK-03)
and `GET /export/movements` (write) — a file exported from one account
re-imports unchanged into another.

| Column | Type | Required | Notes |
| --- | --- | --- | --- |
| `date` | `YYYY-MM-DD` | yes | |
| `amount` | integer, smallest currency unit | yes | sign = direction (negative = debit, positive = credit); must not be zero |
| `currency` | string | yes | must already be registered (`POST /currencies`) |
| `description` | string | no | |
| `category` | string | no | blank -> `other`; see `GET /categories` for the valid set |
| `payment_method` | string | no | blank -> `other`; see `GET /categories` for the valid set |
| `account` | string | no | account name, case-insensitive; blank means unassigned |

The header row is required, matched case-insensitively, and must appear
in this exact column order. `GET /import/movements/spec` mirrors this
list dynamically with the caller's actual registered currencies,
categories, and account names, plus a ready-to-copy template — the
frontend renders that instead of hardcoding this table.
