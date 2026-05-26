Title: feat(money): migrate monetary fields to int64 (cents/kobo) across proto, DB, and services

Summary:
This PR migrates all monetary representations to integer lowest-denomination values (int64 cents/kobo). Changes include:

- Protobuf schema updates: monetary fields converted from `double` to `int64` across services (giveaway, payment, notification, xgateway, compliance, etc.).
- Regenerated Go code via `make proto` and updated `gen/go`.
- Database migrations: updated initial schema to use `BIGINT` for money columns and added `migrations/000003_migrate_amounts_to_bigint.up.sql` to convert existing `NUMERIC(12,2)` data (multiply by 100 and round half-up).
- Repository sweep: updated gateway client structs, payment routing, giveaway handlers, notification handlers, and the NLP parser to use `int64` cents. HTTP JSON endpoints continue to return major-unit floats for human-facing endpoints.

Why:

- Avoid floating-point rounding errors for financial data.
- Simplify arithmetic and reconciliation by storing integer cents in DB and passing integers over RPC.

Migration / rollout checklist (copyable):

1. Backup DB:

```bash
pg_dump "$DATABASE_URL" > backup-before-amount-migration.sql
```

2. Deploy branch to staging and run the conversion migration:

```bash
# run migration tool or psql -f migrations/000003_migrate_amounts_to_bigint.up.sql
```

3. Run the following smoke tests in staging:

- Create a giveaway via `xgateway` → ensure `giveaway` record has integer cents in DB.
- Fund escrow via the sandbox gateway and ensure `CheckEscrowFunded` returns cents.
- Trigger draw and payouts, verify `giveaway_winners.amount` are ints and payouts reconcile correctly.

4. If everything is green, schedule production maintenance and repeat steps.

Notes:

- The branch `feature/int64-amounts` contains the changes and is pushed to origin. Create a PR from it: https://github.com/Mosesedem/bot-x/pull/new/feature/int64-amounts
- `golangci-lint` was not installed in CI runner during local checks; please ensure CI includes linting.

Rollback plan:

- If migration issues are found, restore DB from `backup-before-amount-migration.sql` and revert the deployment.

Testing:

- Unit tests pass/builds succeed locally. Integration tests (testcontainers/docker) remain to be added and should be executed in staging.

Signed-off-by: repo maintainer
