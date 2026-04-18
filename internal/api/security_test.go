package api

// TODO(#83): Security-critical tests that should be written for this PR:
//
// - TestAskCoordTask_OnlyAssigneeOrAdmin (#35)
//     Verify that a non-assignee, non-admin user gets 403 when calling AskCoordTask.
//
// - TestAnswerCoordTask_OnlyCreatorOrAdmin (#35)
//     Verify that a non-creator, non-admin user gets 403 when calling AnswerCoordTask.
//
// - TestVerifyCoordTask_RequiresNewAudit (#36)
//     Verify that VerifyCoordTask returns 409 when no audit completed after task submission.
//
// - TestLoginEmailNormalization (#37)
//     Verify that "Admin@Example.COM" matches a user with email "admin@example.com".
//
// - TestCreateUserEmailNormalization (#37)
//     Verify that email is stored lowercased and duplicate detection is case-insensitive.
//
// - TestUpdateUserRoleValidation (#42)
//     Verify that UpdateUser rejects roles other than "admin" or "member".
//
// - TestGeoPublishRequiresApproval (#48)
//     Verify that GeoPublishArticle rejects articles with review_status != "approved".
//
// - TestDeleteUserCascade (#51)
//     Verify that deleting a user also removes user_oauth_config, user_oauth_tokens,
//     user_telegram, and user_bing rows.
//
// - TestCreateSiteConflictMessage (#90)
//     Verify that the UNIQUE constraint error message is "你已有一个该域名的站点".
//
// - TestEscapeHTML (#41)
//     Verify that escapeHTML replaces &, <, > correctly for Telegram HTML mode.
