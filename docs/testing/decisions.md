# Durable-request decision inventory

This is a reviewed case mapping, not automated MC/DC measurement. Ordinary Go
coverage measures statements. Generated/third-party code is outside this inventory.
A listed test is an implemented case, not a passing claim: final results belong in
TESTING.md and the implementation acceptance record.

| Production decision | Explicit cases / independence | Status |
| --- | --- | --- |
| Identifier byte count, separators, lowercase hex | T01Identifiers valid IDs versus malformed each byte, short/long, uppercase | Executed green: core T01–T04 |
| Name minimum/maximum, endpoint/inner alphabet | T01ParameterBoundaries minimum/maximum, empty/long, initial/final hyphen, uppercase, Unicode, underscore | Executed green: core T01–T04 |
| Image bounds/alphabet | T01ParameterBoundaries minimum/maximum, empty/long/path/Unicode | Executed green: core T01–T04 |
| CPU/memory/disk ranges | T01ParameterBoundaries min/max and adjacent out-of-range; memory overflow | Executed green: core T01–T04 |
| NewRequest validation exits | T01NewRequestValidation invalid request ID, candidate ID and parameters | Executed green: core T01–T04 |
| Canonicalization validation | T02CanonicalLiteral valid and invalid, literal bytes/digest, all five changed fields | Executed green: core T01–T04 |
| Admission HasRequest | T03 C01/C02 (later absent-request predicates not evaluated in C02) | Executed green: core T01–T04 |
| Admission parameter equality | T03 C02/C03 with same fingerprint | Executed green: core T01–T04 |
| Admission fingerprint equality | T03 C02/C04 with same parameters | Executed green: core T01–T04 |
| Admission NameTaken | T03 C01/C05 after no request | Executed green: core T01–T04 |
| Admission Used == Limit | T03 C01/C06 after no request/name | Executed green: core T01–T04 |
| Admission retry precedence | T03 C07 existing at full capacity and occupied name | Executed green: core T01–T04 |
| Admission invariant limit/count/lookup | T03AdmissionInvariants versus valid C01/C02 | Executed green: core T01–T04 |
| Step event, phase and correlation | T04Transitions; T04TransitionInvariants duplicate submit, pending phase, wrong effect, unsupported event | Executed green: core T01–T04 |
| Effect ID and current epoch compound guards | T04EffectIdentityConditions nonempty epoch/sequence independence; equal/unequal epoch in T04FreshStateAndEpochRouting | Cases implemented; focused rerun pending after epoch-routing correction |
| Unknown transition | T04Transitions every outcome; Unknown versus known failure | Executed green: core T01–T04 |
| Outcome enum, record publication and identity | T04TransitionInvariants invalid enum, failure with record, each success relationship independently invalid | Executed green: core T01–T04 |
| Epoch routing | T04FreshStateAndEpochRouting equal versus unequal epoch; S08 old completion after fresh constructor | Executed green: core T01–T04 |
| Wire required/type/duplicate/unknown/trailing/size checks | T01WireStrictness and HTTPInvalidHasNoEffects | Cases implemented; branch audit pending implementation |
| Service ownership, capacity, cancellation, shutdown | T05 tests; fatal worker subprocess; S07/S08 | Cases implemented; branch audit pending implementation |
| HTTP outcomes, read found/error, readiness | T06HTTPOutcomes/ReadClassifications/RealRoundTrip | Cases implemented; branch audit pending implementation |
| CLI command, validation, ID emission, status/failure | T06CLI tests against injected client and local HTTP server | Cases implemented; branch audit pending implementation |
| Config parsing and cross-field constraints | T06ConfigDefaultsAndBounds | Cases implemented; branch audit pending implementation |
| Host file ownership | T08ProcessLock (actual child contention, release, preserved inode) | Cases implemented; branch audit pending implementation |
| Transaction gate/admission/insert/commit/rollback | P01–P07, P10/P11 | Real fixture cases implemented; execution pending |
| Retryable confirmed abort and bounded attempts | P08 real server deadlock; classifier/attempt cases | Real fault case implemented; adapter retry tests pending |
| Migration checksum/version/gate/constraints | P11MigrationGuards | Cases implemented; execution pending |

The core/simulator chunk includes T01–T04 and S01–S10. Rows for boundary and
storage adapters describe follow-on tests currently under development and are not
coverage claims for this chunk.

S01–S10 passed with the production core. For the mutation check, temporarily
returning the retry candidate instead of the persisted Existing winner made S01
fail with `success published before matching durability`. Restoring the exact
production source made the entire simulation suite pass again.

Before acceptance, audit the implemented Go conditions, add missing feasible
independence pairs, explain genuinely infeasible branches, and replace the pending
statuses with observed evidence. Error injection must reach the real boundary it
claims; compile errors, fixture failures and skips contribute no evidence.
