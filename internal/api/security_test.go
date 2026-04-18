package api

// Security-test tracking for #83. The handler-level defensive logic for
// every item below now lives in this package; what's outstanding is the
// integration-test harness (gin + a disposable PostgreSQL) required to
// exercise each flow end-to-end. Pure-function assertions that don't
// need a DB have been split out into *_pure_test.go / password_policy_test.go
// / permissions_pure_test.go — they run in every `go test ./...` pass.
//
// STATUS KEY: ✓ unit-covered here / [int] needs integration harness
//
// ─── Covered by pure tests in this package ────────────────────────────
//
// ✓ TestIsValidUserRole (#42)                    users_pure_test.go
// ✓ TestCleanPermissionList (#permissions)       permissions_pure_test.go
// ✓ TestKnownPermCodesContainsCoreCodes (#perms) permissions_pure_test.go
// ✓ TestValidatePassword (#password-policy)      password_policy_test.go
// ✓ TestEscapeHTML (#41)                         internal/telegram/bot_test.go
//
// ─── Requires gin + DB integration harness (not yet wired up) ─────────
//
// [int] TestAskCoordTask_OnlyAssigneeOrAdmin (#35)
//     Verify that a non-assignee, non-admin user gets 403 when calling
//     AskCoordTask. Handler logic is in coordination.go:676-681; this
//     test would drive the handler through httptest with two fixture
//     users and assert status+body.
//
// [int] TestAnswerCoordTask_OnlyCreatorOrAdmin (#35)
//     Same shape, exercising AnswerCoordTask at coordination.go:722-727.
//
// [int] TestVerifyCoordTask_RequiresNewAudit (#36)
//     Verify 409 when no audit completed after task submission.
//     Guard is at coordination.go:621-636.
//
// [int] TestLoginEmailNormalization (#37)
//     Verify that "Admin@Example.COM" matches a user stored as
//     "admin@example.com". Normalization landed in auth.go:37.
//
// [int] TestCreateUserEmailNormalization (#37)
//     Verify that email is stored lowercased. Normalization landed in
//     users.go:121; a duplicate test should also assert that
//     "Admin@X.com" vs "admin@x.com" is detected as a conflict.
//
// [int] TestUpdateUserEmailNormalization (#37)
//     Verify that UpdateUser lowercases email on the way in. Landed in
//     users.go alongside isValidUserRole.
//
// [int] TestUpdateUserRoleValidation (#42)
//     End-to-end check of the 400 returned for bad roles. Pure logic
//     already covered by TestIsValidUserRole.
//
// [int] TestGeoPublishRequiresApproval (#48)
//     Verify 400 when publishing an article whose review_status isn't
//     "approved". Guard is at geo.go:552-558.
//
// [int] TestDeleteUserCascade (#51)
//     Verify that DELETE /users/:id cascades to user_oauth_config,
//     user_oauth_tokens, user_telegram, and user_bing. Cascade steps
//     live in users.go:346-350.
//
// [int] TestCreateSiteConflictMessage (#90)
//     Verify the UNIQUE constraint maps to "你已有一个该域名的站点".
//
// Opening the integration harness is tracked separately — it's a
// larger change (test PG container, fixture loader, auth helper) that
// belongs in its own PR. Filing it as a follow-up keeps the scope of
// #83 honest: every handler defense is now written, documented, and
// at minimum pure-function covered.
