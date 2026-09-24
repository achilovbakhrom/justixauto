// Conventional Commits (lefthook commit-msg). Subjects may start with a proper
// noun or acronym ("feat(api): OpenAPI spec ..."), so the case rule is off.
export default {
  extends: ['@commitlint/config-conventional'],
  rules: { 'subject-case': [0] },
};
